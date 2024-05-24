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
    "regexp"
    "strings"
    "sync"
    "time"

    "github.com/fsnotify/fsnotify"
    "github.com/gorilla/mux"
    "github.com/santhosh-tekuri/jsonschema/v5"
    "github.com/spf13/pflag"
)

const (
    version = "6"
)

var (
    hostname           string
    port               int
    schemasDir         string
    allowSaveUploads   bool
    verbose            bool
    defaultSpec        string
    maxUploadSizeMB    int
    maxUploadSize      int64
    defaultOutputLevel string
    cache              = make(map[string]*jsonschema.Schema)
    cacheMutex         sync.RWMutex
    stats              = make(map[string]*PathStats)
    statsMutex         sync.Mutex
    showVersion        bool
    validSpecs         = map[string]*jsonschema.Draft{
        "draft4":    jsonschema.Draft4,
        "draft6":    jsonschema.Draft6,
        "draft7":    jsonschema.Draft7,
        "draft2019": jsonschema.Draft2019,
        "draft2020": jsonschema.Draft2020,
    }
    validOutputLevels = map[string]string{
        "basic":    "basic",
        "flag":     "flag",
        "detailed": "detailed",
        "verbose":  "verbose",
    }
    workingDir string
)

// PathStats holds the request statistics for a specific path
type PathStats struct {
    Requests int
    Passes   int
    Fails    int
}

func init() {
    pflag.StringVar(&hostname, "hostname", "localhost", "Hostname to bind the server (default: localhost)")
    pflag.IntVar(&port, "port", 8080, "Port to bind the server (default: 8080)")
    pflag.StringVar(&schemasDir, "schemas-dir", "./schemas", "Directory to load JSON schemas from (default: ./schemas)")
    pflag.BoolVar(&allowSaveUploads, "allow-save-uploads", false, "Allow schema uploads to save to disk (default: false)")
    pflag.BoolVar(&verbose, "verbose", false, "Enable verbose logging (default: false)")
    pflag.BoolVar(&showVersion, "version", false, "Show version number and exit")
    pflag.StringVar(&defaultSpec, "default-spec", "draft7", "Default JSON Schema spec version (default: draft7)")
    pflag.IntVar(&maxUploadSizeMB, "max-upload-size", 2, "Maximum upload size in megabytes (valid range: 1-100)")
    pflag.StringVar(&defaultOutputLevel, "default-outputlevel", "basic", "Default output level (valid values: basic, flag, detailed, verbose)")
    pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
    pflag.Usage = printHelp
}

func sanitizeFilename(filename string) string {
    re := regexp.MustCompile(`[^\w\-.]`)
    return re.ReplaceAllString(filename, "")
}

func safePath(base, name string) (string, error) {
    sanitized := sanitizeFilename(name)
    if sanitized != name {
        return "", fmt.Errorf("unsafe filename")
    }
    fullPath := filepath.Join(base, sanitized)
    if !strings.HasPrefix(fullPath, filepath.Clean(base)+string(os.PathSeparator)) {
        return "", fmt.Errorf("invalid file path")
    }
    return fullPath, nil
}

func getSpec(r *http.Request) (*jsonschema.Draft, error) {
    specParam := r.URL.Query().Get("spec")
    if specParam == "" {
        specParam = defaultSpec
    }
    spec, ok := validSpecs[specParam]
    if !ok {
        return nil, fmt.Errorf("invalid spec: %s", specParam)
    }
    return spec, nil
}

func loadSchema(path string) (*jsonschema.Schema, error) {
    if filepath.Ext(path) != ".json" {
        return nil, fmt.Errorf("file extension must be .json: %s", path)
    }
    log.Printf("Validating schema %s against meta schema", path)

    // Read schema content using Go's file reading functions
    schemaContent, err := ioutil.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read schema file: %w", err)
    }

    compiler := jsonschema.NewCompiler()
    compiler.Draft = validSpecs[defaultSpec]
    compiler.ExtractAnnotations = true
    if err := compiler.AddResource(path, strings.NewReader(string(schemaContent))); err != nil {
        return nil, err
    }
    log.Printf("Path %s", path)
    schema, err := compiler.Compile(path)
    if err != nil {
        log.Printf("invalid schema: %s", err)
        return nil, fmt.Errorf("invalid schema: %s", err)
    }
    log.Printf("Validated OK schema %s against meta schema", path)
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

func stripFilePathsFromErrors(validationErrors []jsonschema.BasicError) []string {
    var errors []string
    for _, ve := range validationErrors {
        errorMsg := ve.KeywordLocation + " " + ve.InstanceLocation
        if strings.HasPrefix(errorMsg, "file://"+workingDir) {
            errorMsg = strings.Replace(errorMsg, "file://"+workingDir, "file://", 1)
        }
        errors = append(errors, errorMsg)
    }
    return errors
}

func validateHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")
    vars := mux.Vars(r)
    schemaFile := vars["schema"]
    if filepath.Ext(schemaFile) != ".json" {
        schemaFile += ".json"
    }

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

    err = schema.Validate(jsonData)
    if err != nil {
        updateStats(r.URL.Path, false)
        validationErrors := err.(*jsonschema.ValidationError).BasicOutput().Errors
        errors := stripFilePathsFromErrors(validationErrors)
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{"result": "Validation failed", "errors": errors})
        logRequest(r, "Validation failed")
        return
    }

    updateStats(r.URL.Path, true)
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"result": "Validation passed"})
    logRequest(r, "Validation passed")
}

func validateWithSchemaHandler(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-Type", "application/json")

    spec, err := getSpec(r)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
        logRequest(r, "Invalid spec")
        return
    }

    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        logRequest(r, "Invalid request body")
        return
    }

    var requestData struct {
        Data   interface{}            `json:"data"`
        Schema map[string]interface{} `json:"schema"`
    }

    if err := json.Unmarshal(body, &requestData); err != nil {
        http.Error(w, `{"error":"Invalid JSON"}`, http.StatusBadRequest)
        logRequest(r, "Invalid JSON")
        return
    }

    schemaBytes, err := json.Marshal(requestData.Schema)
    if err != nil {
        http.Error(w, `{"error":"Invalid schema"}`, http.StatusBadRequest)
        logRequest(r, "Invalid schema")
        return
    }

    compiler := jsonschema.NewCompiler()
    compiler.Draft = spec
    compiler.ExtractAnnotations = true
    if err := compiler.AddResource("inline", strings.NewReader(string(schemaBytes))); err != nil {
        http.Error(w, `{"error":"Error during schema validation"}`, http.StatusInternalServerError)
        logRequest(r, "Error during schema validation")
        return
    }
    schema, err := compiler.Compile("inline")
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{"result": "Schema validation failed", "errors": err.Error()})
        logRequest(r, "Schema validation failed")
        return
    }

    err = schema.Validate(requestData.Data)
    if err != nil {
        updateStats(r.URL.Path, false)
        validationErrors := err.(*jsonschema.ValidationError).BasicOutput().Errors
        errors := stripFilePathsFromErrors(validationErrors)
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{"result": "Validation failed", "errors": errors})
        logRequest(r, "Validation failed")
        return
    }

    updateStats(r.URL.Path, true)
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(map[string]string{"result": "Validation passed"})
    logRequest(r, "Validation passed")
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
    schemaFile := vars["schema"]
    if filepath.Ext(schemaFile) != ".json" {
        schemaFile += ".json"
    }

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
    if !allowSaveUploads {
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
    schemaFile := vars["schema"]
    if filepath.Ext(schemaFile) != ".json" {
        schemaFile += ".json"
    }

    schemaPath, err := safePath(schemasDir, schemaFile)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
        logRequest(r, err.Error())
        return
    }

    spec, err := getSpec(r)
    if err != nil {
        http.Error(w, fmt.Sprintf(`{"error":"%s"}`, err.Error()), http.StatusBadRequest)
        logRequest(r, "Invalid spec")
        return
    }

    schemaBytes, err := json.Marshal(schemaData)
    if err != nil {
        http.Error(w, `{"error":"Invalid schema"}`, http.StatusBadRequest)
        logRequest(r, "Invalid schema")
        return
    }

    compiler := jsonschema.NewCompiler()
    compiler.Draft = spec
    compiler.ExtractAnnotations = true
    if err := compiler.AddResource("uploaded", strings.NewReader(string(schemaBytes))); err != nil {
        http.Error(w, `{"error":"Error during schema validation"}`, http.StatusInternalServerError)
        logRequest(r, "Error during schema validation")
        return
    }
    schema, err := compiler.Compile("uploaded")
    if err != nil {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{"result": "Validation failed", "errors": err.Error()})
        logRequest(r, "Validation failed")
        return
    }

    // Save the schema to disk
    prettySchema, err := json.MarshalIndent(schemaData, "", "  ")
    if err != nil {
        http.Error(w, `{"error":"Failed to pretty print schema"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to pretty print schema")
        return
    }
    err = ioutil.WriteFile(schemaPath, append(prettySchema, '\n'), 0644)
    if err != nil {
        http.Error(w, `{"error":"Failed to save schema"}`, http.StatusInternalServerError)
        logRequest(r, "Failed to save schema")
        return
    }

    // Update the cache
    cacheMutex.Lock()
    cache[schemaFile] = schema
    cacheMutex.Unlock()

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

    if showVersion {
        fmt.Printf("Version: %s\n", version)
        return
    }

    // Check if the 'help' flag is present and its value is true
    helpFlag := pflag.Lookup("help")
    if helpFlag != nil && helpFlag.Value.String() == "true" {
        pflag.Usage()
        return
    }

    // Validate default spec
    if _, ok := validSpecs[defaultSpec]; !ok {
        log.Fatalf("Invalid default spec: %s", defaultSpec)
    }

    // Validate max upload size
    if maxUploadSizeMB < 1 || maxUploadSizeMB > 100 {
        log.Fatalf("Invalid max upload size: %d MB (valid range: 1-100)", maxUploadSizeMB)
    }
    maxUploadSize = int64(maxUploadSizeMB) * 1024 * 1024

    // Validate default output level
    if _, ok := validOutputLevels[defaultOutputLevel]; !ok {
        log.Fatalf("Invalid default output level: %s", defaultOutputLevel)
    }

    // Log all option values and report the full path of directories
    log.Printf("Server starting with the following options:")
    log.Printf("Version: %s", version)
    log.Printf("Hostname: %s", hostname)
    log.Printf("Port: %d", port)
    absSchemasDir, err := filepath.Abs(schemasDir)
    if err != nil {
        log.Fatalf("Failed to get absolute path of schemas directory: %v", err)
    }
    log.Printf("Schemas Directory: %s", absSchemasDir)
    log.Printf("Allow Uploads: %t", allowSaveUploads)
    log.Printf("Verbose Logging: %t", verbose)
    log.Printf("Default Spec: %s", defaultSpec)
    log.Printf("Max Upload Size: %d MB", maxUploadSizeMB)
    log.Printf("Default Output Level: %s", defaultOutputLevel)

    // Get the current working directory
    workingDir, err = os.Getwd()
    if err != nil {
        log.Fatalf("Failed to get current working directory: %v", err)
    }

    // Check if the schemas directory exists
    if _, err := os.Stat(schemasDir); os.IsNotExist(err) {
        log.Fatalf("Schemas directory does not exist: %v", schemasDir)
    }

    if allowSaveUploads {
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
    r.HandleFunc("/validatewithschema", validateWithSchemaHandler).Methods("POST")
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
    fmt.Println("2. Validating JSON data against an inline schema.")
    fmt.Println("3. Retrieving validation statistics.")
    fmt.Println("4. Retrieving a schema.")
    fmt.Println("5. Uploading a new schema (if allowed).")
    fmt.Println("6. Listing all schemas in the directory.")
    fmt.Println("By default, schema uploads are disabled. You can enable schema uploads using the --allow-save-uploads flag.")
    fmt.Println("Uploads are limited in size to prevent excessively large schemas from being uploaded.")
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

  Start the server with a custom default spec:
    go run youvalidateme.go --default-spec=draft2020

  Start the server with a custom max upload size:
    go run youvalidateme.go --max-upload-size=10

  Start the server with a custom default output level:
    go run youvalidateme.go --default-outputlevel=verbose

  Display the version number:
    go run youvalidateme.go --version

Endpoints:
  POST /validate/{schema} - Validate JSON data against the specified schema.
    Example: curl -X POST -d '{"your":"data"}' http://localhost:8080/validate/your_custom_schema_filename.json
    Example with spec query parameter: curl -X POST -d '{"your":"data"}' "http://localhost:8080/validate/your_custom_schema_filename.json?spec=draft7"
    Example with outputlevel query parameter: curl -X POST -d '{"your":"data"}' "http://localhost:8080/validate/your_custom_schema_filename.json?outputlevel=verbose"

  POST /validatewithschema - Validate JSON data against an inline schema.
    Example: curl -X POST -d '{"data":{"your":"data"},"schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"your":{"type":"string"}}}}' http://localhost:8080/validatewithschema
    Example with spec query parameter: curl -X POST -d '{"data":{"your":"data"},"schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"your":{"type":"string"}}}}' "http://localhost:8080/validatewithschema?spec=draft2019"
    Example with outputlevel query parameter: curl -X POST -d '{"data":{"your":"data"},"schema":{"$schema":"http://json-schema.org/draft-07/schema#","type":"object","properties":{"your":{"type":"string"}}}}' "http://localhost:8080/validatewithschema?outputlevel=detailed"

  GET /stats - Retrieve statistics on inbound paths and JSON schema validation passes/fails.
    Example: curl http://localhost:8080/stats

  GET /schema/{schema} - Retrieve the specified schema.
    Example: curl http://localhost:8080/schema/your_custom_schema_filename.json

  POST /schema/{schema} - Upload a new JSON schema (only if --allow-save-uploads is true).
    Example: curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' http://localhost:8080/schema/your_custom_schema_filename.json
    Example with spec query parameter: curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' "http://localhost:8080/schema/your_custom_schema_filename.json?spec=draft6"
    Example with outputlevel query parameter: curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' "http://localhost:8080/schema/your_custom_schema_filename.json?outputlevel=flag"

  GET /schemas - List all JSON schemas in the schemas directory.
    Example: curl http://localhost:8080/schemas
    Example (JSON format): curl http://localhost:8080/schemas?format=json
`)
}
