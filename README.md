# youvalidateme

Sure, here is a `README.md` for your GitHub repository:

```markdown
# YouValidateMe

YouValidateMe is a Go-based server that validates inbound JSON data against JSON schemas. It works by loading JSON schemas from a specified directory and then listening for incoming HTTP requests to validate JSON data against these schemas.

## Features

- **Validate JSON Data**: Validate inbound JSON data against specified schemas.
- **Retrieve Validation Statistics**: Retrieve statistics on inbound paths and JSON schema validation passes/fails.
- **Schema Management**: Retrieve and upload JSON schemas (if uploads are allowed).
- **List Schemas**: List all available JSON schemas in the specified directory.

## Requirements

- Go 1.15 or later

## Installation

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
./youvalidateme --port 9090 --schemas-dir=/path/to/schemas --allow-uploads
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

```

Feel free to customize the README further to fit any specific details or instructions unique to your project.
