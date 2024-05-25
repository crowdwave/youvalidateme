import http.client
import json

# Constants for server configuration
HOSTNAME = '192.168.0.130'
PORT = 8080
TIMEOUT = 2

schema = {
  "$schema": "http://json-schema.org/draft-07/schema#",
  "type": "object",
  "properties": {
    "name": {
      "type": "string"
    },
    "age": {
      "type": "integer",
      "minimum": 0
    },
    "email": {
      "type": "string",
      "format": "email"
    }
  },
  "required": ["name", "email"]
}

data = {
  "name": "John Doe",
  "age": 30,
  "email": "john.doe@example.com"
}



def validate(schema, data):
    try:
        if data is None:
            return False

        if schema is None:
            return False

        # Define the schema to validate against

        # Combine data and schema into a single request payload
        validate_with_schema_payload = {
            "data": data,
            "schema": schema
        }
        headers = {'Content-Type': 'application/json'}

        # Request to validate the data against the schema
        try:
            conn = http.client.HTTPConnection(HOSTNAME, PORT, timeout=TIMEOUT)
            conn.request('POST', '/validatewithschema', body=json.dumps(validate_with_schema_payload), headers=headers)
            validate_response = conn.getresponse()
            validate_response_data = validate_response.read().decode()
            print(f"Validate Response: {validate_response.status}, {validate_response_data}")
        except Exception as e:
            print(f"Validation request failed: {e}")
            return False
        finally:
            conn.close()

        # Check the response from the server
        if validate_response.status == 200:
            result = json.loads(validate_response_data)
            if result.get("result") == "Validation passed":
                return True
            else:
                return False
        else:
            return False
    except Exception as e:
        print(f"An error occurred: {e}")
        return False


print("Testing validate function")
print(f"{validate(schema, data)}")
