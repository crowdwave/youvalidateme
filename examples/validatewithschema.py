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

        # Combine data and schema into a single request payload
        validate_with_schema_payload = {
            "data": {"channel_name": channel_name},
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

# Example usage:
valid_channel_name1 = "valid_channel123"
valid_channel_name2 = "another_valid_channel"
valid_channel_name3 = "third_valid_channel"

invalid_channel_name_too_long = "a" * 65  # 65 characters, exceeds the max length
invalid_channel_name_invalid_chars = "invalid_channel!@#"
invalid_channel_name_empty = ""
invalid_channel_name_none = None

print("Testing valid channel names:")
print(f"Is '{valid_channel_name1}' valid: {validate_channel_id(valid_channel_name1)}")
print(f"Is '{valid_channel_name2}' valid: {validate_channel_id(valid_channel_name2)}")
print(f"Is '{valid_channel_name3}' valid: {validate_channel_id(valid_channel_name3)}")

print("\nTesting invalid channel names (too long):")
print(f"Is '{invalid_channel_name_too_long}' valid: {validate_channel_id(invalid_channel_name_too_long)}")

print("\nTesting invalid channel names (invalid characters):")
print(f"Is '{invalid_channel_name_invalid_chars}' valid: {validate_channel_id(invalid_channel_name_invalid_chars)}")

print("\nTesting invalid channel name (empty string):")
print(f"Is '{invalid_channel_name_empty}' valid: {validate_channel_id(invalid_channel_name_empty)}")

print("\nTesting invalid channel name (None):")
print(f"Is '{invalid_channel_name_none}' valid: {validate_channel_id(invalid_channel_name_none)}")
