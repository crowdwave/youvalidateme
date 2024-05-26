# YouValidateMe

<p align="center">
  <img src="YOUVALIDATEMELOGO.png" alt="Logo">
</p>

YouValidateMe is a library server for storing and validating against your JSON Schema documents. 

Why? To centralise field validation logic for your applications instead of scattering different validation logic everywhere that might get out of sync.

Features:

- **HTTP server**: YouValidateMe is a special purpose HTTP server focused on helping you create a library of JSON Schema validation documents for your applications, and validating JSON data against them. 
- **Your JSON schema library**: You can upload your JSON Schema documents to the server. 
- **Validates JSON Data**: You can validate JSON data against schemas in your library.
- **Retrieve Validation Statistics**: Retrieve statistics on inbound paths and JSON schema validation passes/fails.
- **Schema Management**: You can get/view JSON schemas in your library. 
- **List Schemas**: You can list all the JSON schemas in your library.
- **Zero config, single binary**: It's a single binary - nothing to install or configure, just run it.
- **Example systemd unit provided**: It's a single binary - nothing to install or configure, just run it.
- **Multiple specs supported** draft4, draft6, draft7, draft2019, draft2020
- **Open Source**: It's written in Golang open source and licensed under the MIT License.

YouValidateMe is based on https://github.com/santhosh-tekuri/jsonschema which is a JSON Schema validator for Go - the output from this library is used to provide the validation results.

Ways to use YouValidateMe:

1. JSON schema documents are stored as plain text JSON files on the server.
2. You can manually copy your JSON schema documents to the server using normal operating system copy commands.
3. You can upload your JSON schema documents to the server via POST using the provided API.
4. You can validate by POSTing JSON data to the filename of one of your schema documents.
5. You can pull schema documents into your code via GET and then validate JSON data using your own language/library.
6. You can validate by submitting both data and schema to the server in a single POST.
7. You can choose which JSON schema draft level to validate against by specifying 'spec' query parameter in the POST request.

## IMPORTANT!!!

YouValidateMe is brand new and is not battle tested (or tested at all!). You should read the Go source code before using it (it's a single file of less than 500 lines, won't take long to read) and you should test it to your satisfaction before relying on it. There is no guarantee at all - use at your own risk.

## How to use it in your applications.

There is a binary file named youvalidateme in this repo which is compiled for Linux AMD64 - you'll need to compile it yourself if you are using other platforms.

There is a Python file in the examples directory that illustrates how you would use YouValidateMe from within your own application.  If you are using a different language then look at the Pyuthon code anyway - it is simple to understand and you can easily transfer the principles to your preferred programming language or ask ChatGPT to convert it.

## Features

- **Validate JSON Data**: Validate inbound JSON data against specified schemas.
- **Retrieve Validation Statistics**: Retrieve statistics on inbound paths and JSON schema validation passes/fails.
- **Schema Management**: Retrieve and upload JSON schemas (if uploads are allowed).
- **List Schemas**: List all available JSON schemas in the specified directory.

## Warning - don't connect this to the public Internet!

This server is not designed for security and should not be connected to the public Internet. It is intended to be used on a private network or on a local server.

## Why must youvalidateme be started with sudo?

On linux (not other platforms) this server must be started with sudo or as root. This is because when the server starts it puts itself in a chroot jail which means it cannot see files outside its working directory. This is why you are required to provide --user and --group on the command line so that the server can change its user and group to a non-root user after it has started.  This is a security feature to prevent the server from being able to access the entire file system.

## To compile it yourself

A binary is provided for 64 bit AMD64 linux but you can compile it yourself for other platforms.

Clone the repository:

```sh
git clone https://github.com/crowdwave/youvalidateme.git
cd youvalidateme
```

Build the server:

# important! this program must be compiled with the CGO_ENABLED=0 flag set or it will not work.

```sh
go get github.com/fsnotify/fsnotify
go get github.com/gorilla/mux
go get github.com/spf13/pflag
go get github.com/dlclark/regexp2
go get github.com/santhosh-tekuri/jsonschema/v5

CGO_ENABLED=0 go build -o youvalidateme youvalidateme.go
```

## Usage

Start the server with default options:

```sh
sudo ./youvalidateme --user ubuntu --group ubuntu
```

**important! This program must be run as root or with sudo or it will not work.**
Start the server with custom options:

```sh
sudo ./youvalidateme --hostname 0.0.0.0 --port 8080 --schemas-dir=/path/to/schemas --allow-save-uploads  --user ubuntu --group ubuntu
```

### Command-line Options

- `--hostname`: Hostname to bind the server (default: `localhost`)
- `--port`: Port to bind the server (default: `8080`)
- `--schemas-dir`: Directory to load JSON schemas from (default: `./schemas`)
- `--allow-save-uploads`: Allow schema uploads (default: `false`)
- `--user`: The user to run the server as
- `--group`: The group to run the server as

## Endpoints
Here's the complete documentation of the package, including the list of all endpoints at the top and detailed descriptions, including optional query parameters for each supported endpoint.

### List of Endpoints

1. **POST /validate/{schema}** - Validate JSON data against the specified schema.
2. **POST /validatewithschema** - Validate JSON data against an inline schema.
3. **GET /stats** - Retrieve statistics on inbound paths and JSON schema validation passes/fails.
4. **GET /schema/{schema}** - Retrieve the specified schema.
5. **POST /schema/{schema}** - Upload a new JSON schema.
6. **GET /schemas** - List all JSON schemas in the schemas directory.

### Endpoint Details

#### 1. POST /validate/{schema}

**Description:**
Validates JSON data against the specified schema file.

**Expected Data Structure:**
The request body should contain the JSON data to be validated.

**Optional Query Parameters:**
- `spec`: Specifies the JSON Schema draft version (e.g., `draft7`). Default is `draft7`.
- `outputlevel`: Specifies the level of detail for the validation output. Valid values are `basic`, `flag`, `detailed`, `verbose`.

**Example:**
```bash
curl -X POST -d '{"your":"data"}' http://localhost:8080/validate/your_custom_schema_filename.json
```
Example with query parameters:
```bash
curl -X POST -d '{"your":"data"}' "http://localhost:8080/validate/your_custom_schema_filename.json?spec=draft7&outputlevel=verbose"
```

#### 2. POST /validatewithschema

**Description:**
Validates JSON data against an inline schema provided in the request body.

**Expected Data Structure:**
The request body should be a JSON object with two fields:
- `data`: The actual JSON data to be validated.
- `schema`: The JSON schema that defines the validation rules for the data.

**Optional Query Parameters:**
- `spec`: Specifies the JSON Schema draft version (e.g., `draft7`). Default is `draft7`.
- `outputlevel`: Specifies the level of detail for the validation output. Valid values are `basic`, `flag`, `detailed`, `verbose`.

**Example:**
```bash
curl -X POST -H "Content-Type: application/json" -d '{
  "data": {
    "your": "data"
  },
  "schema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "properties": {
      "your": {
        "type": "string"
      }
    },
    "required": ["your"]
  }
}' http://localhost:8080/validatewithschema
```
Example with query parameters:
```bash
curl -X POST -H "Content-Type: application/json" -d '{
  "data": {
    "your": "data"
  },
  "schema": {
    "$schema": "http://json-schema.org/draft-07/schema#",
    "type": "object",
    "properties": {
      "your": {
        "type": "string"
      }
    },
    "required": ["your"]
  }
}' "http://localhost:8080/validatewithschema?spec=draft2019&outputlevel=detailed"
```

#### 3. GET /stats

**Description:**
Retrieves statistics on inbound paths and JSON schema validation passes/fails.

**Example:**
```bash
curl http://localhost:8080/stats
```

#### 4. GET /schema/{schema}

**Description:**
Retrieves the specified schema file.

**Example:**
```bash
curl http://localhost:8080/schema/your_custom_schema_filename.json
```

#### 5. POST /schema/{schema}

**Description:**
Uploads a new JSON schema. This endpoint is enabled only if the `--allow-save-uploads` flag is set to true.

**Expected Data Structure:**
The request body should contain the JSON schema to be uploaded.

**Optional Query Parameters:**
- `spec`: Specifies the JSON Schema draft version (e.g., `draft7`). Default is `draft7`.
- `outputlevel`: Specifies the level of detail for the validation output. Valid values are `basic`, `flag`, `detailed`, `verbose`.

**Example:**
```bash
curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' http://localhost:8080/schema/your_custom_schema_filename.json
```
Example with query parameters:
```bash
curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' "http://localhost:8080/schema/your_custom_schema_filename.json?spec=draft6&outputlevel=flag"
```

#### 6. GET /schemas

**Description:**
Lists all JSON schemas in the schemas directory.

**Optional Query Parameters:**
- `format`: Specifies the response format. Valid values are `json` (for JSON format) and any other value (for HTML format).

**Example:**
```bash
curl http://localhost:8080/schemas
```
Example with JSON format:
```bash
curl http://localhost:8080/schemas?format=json
```

### Examples

Examine the Python programs in the examples directory to see how to use YouValidateMe from within your own application.


## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements or bug fixes.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

