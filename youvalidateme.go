package main

import (
    "encoding/json"
    "flag"
    "fmt"
    "html/template"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "path/filepath"
    "sync"

    "github.com/fsnotify/fsnotify"
    "github.com/gorilla/mux"
    "github.com/spf13/pflag"
    "github.com/xeipuuv/gojsonschema"
)

const maxUploadSize = 100 * 1024 // 100K

var (
    hostname       string
    port           int
    schemasDir     string
    allowUploads   bool
    cache          = make(map[string]*gojsonschema.Schema)
    cacheMutex     sync.RWMutex
    stats          = make(map[string]*PathStats)
    statsMutex     sync.Mutex
)

type PathStats struct {
    Requests int
    Passes   int
    Fails    int
}

func init() {
    pflag.StringVar(&hostname, "hostname", "localhost", "Hostname to bind the server (default: localhost)")
    pflag.IntVar(&port, "port", 8080, "Port to bind the server (default: 8080)")
    pflag.StringVar(&schemasDir, "schemas-dir", "./schemas", "Directory to load JSON schemas from (default: ./schemas)")
    pflag.BoolVar(&allowUploads, "allow-uploads", false, "Allow schema uploads (default: false)")
    pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
    pflag.Usage = printHelp
}

func loadSchema(path string) (*gojsonschema.Schema, error) {
    if filepath.Ext(path) != ".json" {
        return nil, fmt.Errorf("file extension must be .json: %s", path)
    }
    schemaLoader := gojsonschema.NewReferenceLoader(fmt.Sprintf("file://%s", path))
    schema, err := gojsonschema.NewSchema(schemaLoader)
    if err != nil {
        return nil, fmt.Errorf("failed to load schema from %s: %v", path, err)
    }
    return schema, nil
}

func loadSchemas() {
    files, err := ioutil.ReadDir(schemasDir)
    if err != nil {
        log.Fatalf("Failed to read schemas directory: %v", err)
    }

    for _, file := range files {
        if filepath.Ext(file.Name()) == ".json" {
            schemaPath := filepath.Join(schemasDir, file.Name())
            schema, err := loadSchema(schemaPath)
            if err != nil {
                log.Printf("Failed to load schema %s: %v", schemaPath, err)
                continue
            }
            cacheMutex.Lock()
            cache[file.Name()] = schema
            cacheMutex.Unlock()
        }
    }
}

func watchSchemas() error {
    watcher, err := fsnotify.NewWatcher()
    if err != nil {
        return fmt.Errorf("failed to create watcher: %v", err)
    }
    defer watcher.Close()

    err = watcher.Add(schemasDir)
    if err != nil {
        return fmt.Errorf("failed to add directory to watcher: %v", err)
    }

    for {
        select {
        case event := <-watcher.Events:
            if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
                schemaPath := event.Name
                if filepath.Ext(schemaPath) == ".json" {
                    schema, err := loadSchema(schemaPath)
                    if err != nil {
                        log.Printf("Failed to reload schema %s: %v", schemaPath, err)
                        continue
                    }
                    cacheMutex.Lock()
                    cache[filepath.Base(schemaPath)] = schema
                    cacheMutex.Unlock()
                    log.Printf("Reloaded schema: %s", schemaPath)
                }
            }
        case err := <-watcher.Errors:
            log.Println("Error watching schemas:", err)
        }
    }
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"

    cacheMutex.RLock()
    schema, found := cache[schemaFile]
    cacheMutex.RUnlock()

    if !found {
        http.Error(w, "Schema not found", http.StatusNotFound)
        return
    }

    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    var jsonData interface{}
    if err := json.Unmarshal(body, &jsonData); err != nil {
        http.Error(w, "Invalid JSON", http.StatusBadRequest)
        return
    }

    documentLoader := gojsonschema.NewGoLoader(jsonData)
    result, err := schema.Validate(documentLoader)
    if err != nil {
        http.Error(w, "Error during validation", http.StatusInternalServerError)
        return
    }

    updateStats(r.URL.Path, result.Valid())

    if result.Valid() {
        w.WriteHeader(http.StatusOK)
        w.Write([]byte("Validation passed"))
    } else {
        w.WriteHeader(http.StatusBadRequest)
        w.Write([]byte("Validation failed: " + fmt.Sprintf("%v", result.Errors())))
    }
}

func updateStats(path string, passed bool) {
    statsMutex.Lock()
    defer statsMutex.Unlock()

    if stats[path] == nil {
        stats[path] = &PathStats{}
    }

    stats[path].Requests++
    if passed {
        stats[path].Passes++
    } else {
        stats[path].Fails++
    }
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
    statsMutex.Lock()
    defer statsMutex.Unlock()

    jsonStats, err := json.Marshal(stats)
    if err != nil {
        http.Error(w, "Error generating stats", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(jsonStats)
}

func schemaHandler(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"

    cacheMutex.RLock()
    _, found := cache[schemaFile]
    cacheMutex.RUnlock()

    if !found {
        http.Error(w, "Schema not found", http.StatusNotFound)
        return
    }

    schemaPath := filepath.Join(schemasDir, schemaFile)
    schemaContent, err := ioutil.ReadFile(schemaPath)
    if err != nil {
        http.Error(w, "Failed to read schema file", http.StatusInternalServerError)
        return
    }

    w.Header().Set("Content-Type", "application/json")
    w.Write(schemaContent)
}

func uploadSchemaHandler(w http.ResponseWriter, r *http.Request) {
    if !allowUploads {
        http.Error(w, "Schema uploads are disabled", http.StatusForbidden)
        return
    }

    if r.ContentLength > maxUploadSize {
        http.Error(w, "Uploaded schema is too large", http.StatusRequestEntityTooLarge)
        return
    }

    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Invalid request body", http.StatusBadRequest)
        return
    }

    var schemaData interface{}
    if err := json.Unmarshal(body, &schemaData); err != nil {
        http.Error(w, "Invalid JSON schema", http.StatusBadRequest)
        return
    }

    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"
    if filepath.Ext(schemaFile) != ".json" {
        http.Error(w, "File extension must be .json", http.StatusBadRequest)
        return
    }
    schemaPath := filepath.Join(schemasDir, schemaFile)

    // Save the schema to disk
    err = ioutil.WriteFile(schemaPath, body, 0644)
    if err != nil {
        http.Error(w, "Failed to save schema", http.StatusInternalServerError)
        return
    }

    // Update the cache
    cacheMutex.Lock()
    cache[schemaFile], err = loadSchema(schemaPath)
    cacheMutex.Unlock()

    if err != nil {
        http.Error(w, "Failed to load schema into cache", http.StatusInternalServerError)
        return
    }

    w.WriteHeader(http.StatusOK)
    w.Write([]byte("Schema uploaded and validated successfully"))
}

func listSchemasHandler(w http.ResponseWriter, r *http.Request) {
    files, err := ioutil.ReadDir(schemasDir)
    if err != nil {
        http.Error(w, "Failed to read schemas directory", http.StatusInternalServerError)
        return
    }

    schemaFiles := []string{}
    for _, file := range files {
        if filepath.Ext(file.Name()) == ".json" {
            schemaFiles = append(schemaFiles, file.Name())
        }
    }

    format := r.URL.Query().Get("format")
    if format == "json" {
        jsonResponse, err := json.Marshal(schemaFiles)
        if err != nil {
            http.Error(w, "Failed to generate JSON response", http.StatusInternalServerError)
            return
        }
        w.Header().Set("Content-Type", "application/json")
        w.Write(jsonResponse)
    } else {
        w.Header().Set("Content-Type", "text/html")
        tmpl := template.Must(template.New("schemas").Parse(`
            <!DOCTYPE html>
            <html>
            <head>
                <title>Schemas</title>
            </head>
            <body>
                <h1>Schemas</h1>
                <ul>
                {{range .}}
                    <li>{{.}}</li>
                {{end}}
                </ul>
            </body>
            </html>`))
        tmpl.Execute(w, schemaFiles)
    }
}

func checkSchemasDirWritable() error {
    testFile := filepath.Join(schemasDir, "test_write")
    if err := ioutil.WriteFile(testFile, []byte("test"), 0644); err != nil {
        return err
    }
    return os.Remove(testFile)
}

func main() {
    pflag.Parse()

    // Check if the 'help' flag is present and its value is true
    helpFlag := pflag.Lookup("help")
    if helpFlag != nil && helpFlag.Value.String() == "true" {
        pflag.Usage()
        return
    }

    // Log all option values and report the full path of directories
    log.Printf("Server starting with the following options:")
    log.Printf("Hostname: %s", hostname)
    log.Printf("Port: %d", port)
    absSchemasDir, err := filepath.Abs(schemasDir)
    if err != nil {
        log.Fatalf("Failed to get absolute path of schemas directory: %v", err)
    }
    log.Printf("Schemas Directory: %s", absSchemasDir)
    log.Printf("Allow Uploads: %t", allowUploads)

    // Check if the schemas directory exists
    if _, err := os.Stat(schemasDir); os.IsNotExist(err) {
        log.Fatalf("Schemas directory does not exist: %v", schemasDir)
    }

    if allowUploads {
        if err := checkSchemasDirWritable(); err != nil {
            log.Fatalf("Schemas directory is not writable: %v", err)
        }
    }

    // Load initial schemas
    loadSchemas()

    // Start watching for schema changes
    go func() {
        if err := watchSchemas(); err != nil {
            log.Fatalf("Error watching schemas: %v", err)
        }
    }()

    r := mux.NewRouter()
    r.HandleFunc("/validate/{schema}", validateHandler).Methods("POST")
    r.HandleFunc("/stats", statsHandler).Methods("GET")
    r.HandleFunc("/schema/{schema}", schemaHandler).Methods("GET")
    r.HandleFunc("/schema/{schema}", uploadSchemaHandler).Methods("POST")
    r.HandleFunc("/schemas", listSchemasHandler).Methods("GET")

    addr := fmt.Sprintf("%s:%d", hostname, port)
    log.Printf("Starting server on %s\n", addr)
    log.Fatal(http.ListenAndServe(addr, r))
}

func printHelp() {
    fmt.Println("This server validates inbound JSON data against JSON schemas.")
    fmt.Println("It works by loading JSON schemas from a specified directory, and then listening for incoming HTTP requests to validate JSON data against these schemas.")
    fmt.Println("The server supports the following operations:")
    fmt.Println("1. Validating JSON data against a schema.")
    fmt.Println("2. Retrieving validation statistics.")
    fmt.Println("3. Retrieving a schema.")
    fmt.Println("4. Uploading a new schema (if allowed).")
    fmt.Println("5. Listing all schemas in the directory.")
    fmt.Println("By default, schema uploads are disabled. You can enable schema uploads using the --allow-uploads flag.")
    fmt.Println("Uploads are limited to 100K in size to prevent excessively large schemas from being uploaded.")
    fmt.Println("For the validate and get schema operations, the schema file must have a .json extension and be located in the specified schemas directory.")

    fmt.Fprintf(flag.CommandLine.Output(), "\nUsage of %s:\n", filepath.Base(os.Args[0]))
    fmt.Println("Command-line options:")
    pflag.PrintDefaults()
    fmt.Println(`
Examples:
  Start the server with default options:
    go run youvalidateme.go

  Start the server with a custom port and schemas directory:
    go run youvalidateme.go --port 9090 --schemas-dir=/path/to/schemas

Endpoints:
  POST /validate/{schema} - Validate JSON data against the specified schema.
    Example: curl -X POST -d '{"your":"data"}' http://localhost:8080/validate/your_schema.json

  GET /stats - Retrieve statistics on inbound paths and JSON schema validation passes/fails.
    Example: curl http://localhost:8080/stats

  GET /schema/{schema} - Retrieve the specified schema.
    Example: curl http://localhost:8080/schema/your_schema.json

  POST /schema/{schema} - Upload a new JSON schema (only if --allow-uploads is true).
    Example: curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' http://localhost:8080/schema/your_schema.json

  GET /schemas - List all JSON schemas in the schemas directory.
    Example: curl http://localhost:8080/schemas
    Example (JSON format): curl http://localhost:8080/schemas?format=json
`)
}
