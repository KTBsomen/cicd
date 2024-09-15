import hmac
import hashlib
import random
import requests
import json

def generate_signature(secret, payload):
    """Generate HMAC SHA256 signature."""
    hmac_digest = hmac.new(secret.encode(), payload.encode(), hashlib.sha256).hexdigest()
    return f'sha256={hmac_digest}'

# Configuration
url = 'http://localhost:8000'  # Replace with your server URL if different
secret = '1234'
payload = {
    "ref": "refs/heads/main",
    "head_commit": {
        "id": "abcbbm5"+str(random.randint(1,1222222222222)),
        "author": {
            "name": "Test Author mk"
        }
    }
}

# Convert payload to JSON string
payload_json = json.dumps(payload)

# Generate the signature
signature = generate_signature(secret, payload_json)

# Send the POST request
headers = {
    'Content-Type': 'application/json',
    'X-Hub-Signature-256': signature
}

response = requests.post(url, headers=headers, data=payload_json)

# Print response from the server
print(f"Status Code: {response.status_code}")
print(f"Response Text: {response.text}")
