import os
import requests

API_KEY = os.getenv("EMAIL_API_KEY")
URL = "https://api.resend.com/emails"

HEADERS = {
    "Authorization": f"Bearer {API_KEY}",
    "Content-Type": "application/json"
}

payload = {
    "from": "Suitee <update@suitee.com.br>",
    "to": ["test@example.com"],
    "template_id": "fc0c800d-ebad-46bf-96f5-279b9f08ef5b",
    "variables": {
        "user": "John Doe",
        "link": "https://example.com/dashboard"
    }
}

res = requests.post(URL, headers=HEADERS, json=payload)
print(res.status_code, res.text)
