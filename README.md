# YouValidateMe

<p align="center">
  <img src="YOUVALIDATEMELOGO.png" alt="Logo">
</p>

YouValidateMe is a library server for storing and validating against your JSON Schema documents. 

Why? To centralise field validation logic for your applications instead of scattering different validation logic everywhere that might get out of sync.

Features:

- **HTTP server**: YouValidateMe is a special purpose HTTP server focused on heling you create a library of JSON Schema validation documents for your applications, and validating JSON data against them. 
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

1: JSON schema documents are stored as plain text JSON files on the server.
2: You can manually copy your JSON schema documents to the server using normal operating system copy commands.
3: You can upload your JSON schema documents to the server via POST using the provided API.
4: You can validate by POSTing JSON data to the filename of one of your schema documents.
5: You can pull schema documents into your code via GET and then validate JSON data using your own language/library.
6: You can validate by submitting both data and schema to the server in a single POST.
7: You can choose which JSON schema draft level to validate against by specifying 'spec' query parameter in the POST request.

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

## To compile it yourself

A binary is provided for 64 bit AMD64 linux but you can compile it yourself for other platforms.

Clone the repository:

```sh
git clone https://github.com/yourusername/youvalidateme.git
cd youvalidateme
```

Build the server:

```sh
go build -o youvalidateme youvalidateme.go
```

## Usage

Start the server with default options:

```sh
./youvalidateme
```

Start the server with custom options:

```sh
./youvalidateme --port 8080 --schemas-dir=/path/to/schemas --allow-uploads
```

### Command-line Options

- `--hostname`: Hostname to bind the server (default: `localhost`)
- `--port`: Port to bind the server (default: `8080`)
- `--schemas-dir`: Directory to load JSON schemas from (default: `./schemas`)
- `--allow-uploads`: Allow schema uploads (default: `false`)

## Endpoints

### Validate JSON Data

Validate JSON data against the specified schema.

```sh
curl -X POST -d '{"your":"data"}' http://localhost:8080/validate/your_schema.json
```

### Retrieve Validation Statistics

Retrieve statistics on inbound paths and JSON schema validation passes/fails.

```sh
curl http://localhost:8080/stats
```

### Retrieve a Schema

Retrieve the specified schema.

```sh
curl http://localhost:8080/schema/your_schema.json
```

### Upload a New Schema

Upload a new JSON schema (only if uploads are allowed).

```sh
curl -X POST -d '{"$schema":"http://json-schema.org/draft-07/schema#","title":"Example","type":"object","properties":{"example":{"type":"string"}}}' http://localhost:8080/schema/your_schema.json
```

### List All Schemas

List all JSON schemas in the schemas directory.

```sh
curl http://localhost:8080/schemas
```

List all JSON schemas in the schemas directory in JSON format.

```sh
curl http://localhost:8080/schemas?format=json
```

## Development

### Running the Server

To run the server locally for development purposes, use:

```sh
go run youvalidateme.go
```

### Testing

To test the server, you can use the provided curl commands to interact with the various endpoints.

## Contributing

Contributions are welcome! Please open an issue or submit a pull request for any improvements or bug fixes.

## License

This project is licensed under the MIT License. See the [LICENSE](LICENSE) file for details.

