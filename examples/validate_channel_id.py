import http.client
import json

# Constants for server configuration
HOSTNAME = '192.168.0.130'
PORT = 8080
TIMEOUT = 2

def validate_channel_id(channel_name):
    try:
        if channel_name is None:
            return False

        # Define the schema to validate against
        schema = {
            "$schema": "http://json-schema.org/draft-07/schema#",
            "type": "object",
            "required": ["channel_name"],
            "properties": {
                "channel_name": {
                    "type": "string",
                    "maxLength": 64,
                    "pattern": "^[a-zA-Z0-9-_' ]*$"
                }
            },
            "additionalProperties": False
        }

        # Define the schema path and data
        schema_path = 'validate_channel_id.json'
        headers = {'Content-Type': 'application/json'}

        # First request to save the schema to the server
        try:
            conn = http.client.HTTPConnection(HOSTNAME, PORT, timeout=TIMEOUT)
            conn.request('POST', f'/schema/{schema_path}', body=json.dumps(schema), headers=headers)
            save_schema_response = conn.getresponse()
            save_schema_response_data = save_schema_response.read().decode()
            print(f"Save Schema Response: {save_schema_response.status}, {save_schema_response_data}")
        except Exception as e:
            print(f"Save schema request failed: {e}")
            return False
        finally:
            conn.close()

        if save_schema_response.status != 200:
            return False

        # Second request to validate the data against the schema
        validate_data = {"channel_name": channel_name}
        try:
            conn = http.client.HTTPConnection(HOSTNAME, PORT, timeout=TIMEOUT)
            conn.request('POST', f'/validate/{schema_path}', body=json.dumps(validate_data), headers=headers)
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

# Example usage:
channel_name = "valid_channel123"
is_valid = validate_channel_id(channel_name)
print(is_valid)
