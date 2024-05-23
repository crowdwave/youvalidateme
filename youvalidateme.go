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
    "time"

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
    verbose        bool
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
    pflag.BoolVar(&verbose, "verbose", false, "Enable verbose logging (default: false)")
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

func logRequest(r *http.Request, outcome string) {
    if verbose {
        log.Printf("[%s] %s %s - %s", time.Now().Format(time.RFC3339), r.Method, r.URL.Path, outcome)
    }
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"

    cacheMutex.RLock()
    schema, found := cache[schemaFile]
    cacheMutex.RUnlock()

    if !found {
        http.Error(w, `{"error":"Schema not found"}`, http.StatusNotFound)
        logRequest(r, "Schema not found")
        return
    }

    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        logRequest(r, "Invalid request body")
        return
    }

    var jsonData interface{}
    if err := json.Unmarshal(body, &jsonData); err != nil {
        http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
        logRequest(r, "Invalid JSON")
        return
    }

    documentLoader := gojsonschema.NewGoLoader(jsonData)
    result, err := schema.Validate(documentLoader)
    if err != nil {
        http.Error(w, `{"error":"Error during validation"}`, http.StatusInternalServerError)
        logRequest(r, "Error during validation")
        return
    }

    updateStats(r.URL.Path, result.Valid())

    if result.Valid() {
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]string{"result": "Validation passed"})
        logRequest(r, "Validation passed")
    } else {
        w.WriteHeader(http.StatusBadRequest)
        errors := []string{}
        for _, err := range result.Errors() {
            errors = append(errors, err.String())
        }
        json.NewEncoder(w).Encode(map[string]interface{}{"result": "Validation failed", "errors": errors})
        logRequest(r, "Validation failed")
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
    w.Header().Set("Content-Type", "application/json")
    statsMutex.Lock()
    defer statsMutex.Unlock()

    jsonStats, err := json.Marshal(stats)
    if err != nil {
        http.Error(w, `{"error":"Error generating stats"}`, http.StatusInternalServerError)
        logRequest(r, "Error generating stats")
        return
    }

    w.Write(jsonStats)
    logRequest(r, "Stats retrieved")
}

func schemaHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"

    cacheMutex.RLock()
    _, found := cache[schemaFile]
    cacheMutex.RUnlock()

    if !found {
        http.Error(w, `{"error":"Schema not found"}`, http.StatusNotFound)
        logRequest(r, "Schema not found")
        return
    }

    schemaPath := filepath.Join(schemasDir, schemaFile)
    schemaContent, err := ioutil.ReadFile(schemaPath)
    if err != nil {
        http.Error(w, `{"error":"Failed to read schema file"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to read schema file")
        return
    }

    w.Write(schemaContent)
    logRequest(r, "Schema retrieved")
}

func uploadSchemaHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    if !allowUploads {
        http.Error(w, `{"error":"Schema uploads are disabled"}`, http.StatusForbidden)
        logRequest(r, "Schema uploads are disabled")
        return
    }

    if r.ContentLength > maxUploadSize {
        http.Error(w, `{"error":"Uploaded schema is too large"}`, http.StatusRequestEntityTooLarge)
        logRequest(r, "Uploaded schema is too large")
        return
    }

    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        logRequest(r, "Invalid request body")
        return
    }

    var schemaData interface{}
    if err := json.Unmarshal(body, &schemaData); err != nil {
        http.Error(w, `{"error":"Invalid JSON schema"}`, http.StatusBadRequest)
        logRequest(r, "Invalid JSON schema")
        return
    }

    vars := mux.Vars(r)
    schemaFile := vars["schema"] + ".json"
    if filepath.Ext(schemaFile) != ".json" {
        http.Error(w, `{"error":"File extension must be .json"}`, http.StatusBadRequest)
        logRequest(r, "File extension must be .json")
        return
    }
    schemaPath := filepath.Join(schemasDir, schemaFile)

    // Save the schema to disk
    err = ioutil.WriteFile(schemaPath, body, 0644)
    if err != nil {
        http.Error(w, `{"error":"Failed to save schema"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to save schema")
        return
    }

    // Update the cache
    cacheMutex.Lock()
    cache[schemaFile], err = loadSchema(schemaPath)
    cacheMutex.Unlock()

    if err != nil {
        http.Error(w, `{"error":"Failed to load schema into cache"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to load schema into cache")
        return
    }

    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"result": "Schema uploaded and validated successfully"})
    logRequest(r, "Schema uploaded and validated successfully")
}

func listSchemasHandler(w http.ResponseWriter, r *http.Request) {
    files, err := ioutil.ReadDir(schemasDir)
    if err != nil {
        http.Error(w, `{"error":"Failed to read schemas directory"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to read schemas directory")
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
            http.Error(w, `{"error":"Failed to generate JSON response"}`, http.StatusInternalServerError)
            logRequest(r, "Failed to generate JSON response")
            return
        }
        w.Header().Set("Content-Type", "application/json")
        w.Write(jsonResponse)
        logRequest(r, "Schema list retrieved (JSON format)")
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
        logRequest(r, "Schema list retrieved (HTML format)")
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
    log.Printf("Verbose Logging: %t", verbose)

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
    fmt.Println("Verbose logging can be enabled using the --verbose flag to log all inbound requests with date, method, path, and outcome.")

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
