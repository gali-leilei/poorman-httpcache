# /// script
# requires-python = ">=3.12"
# dependencies = [
#     "requests<3",
# ]
# ///
import os
import requests

JINA_BASE_URL = os.getenv("JINA_BASE_URL", "https://r.jina.ai")
JINA_API_KEY = os.getenv("JINA_API_KEY", "")
INTERNAL_KEY = os.getenv("INTERNAL_KEY", "")
SERVICE_DOMAIN = os.getenv("SERVICE_DOMAIN", "http://127.0.0.1:8080")

def debug_print(response: requests.Response) -> None:
    print(f"Status Code: {response.status_code}")
    print(f"Headers: {response.headers}")
    print(f"Content: {response.text}")

def run_test(base_url: str, api_key: str) -> None:
    urls = [
        # "https://www.example.com",
        # "https://news.ycombinator.com/news",
        # "https://sans-io.readthedocs.io",
        # problematic urls
        "https://www.bbc.co.uk/writers/documents/doctor-who-s9-ep11-heaven-sent-steven-moffat.pdf",
    ]
    for url in urls:
        # Use Jina.ai reader API to convert URL to LLM-friendly text
        jina_url = f"{base_url}/{url}"
        # jina_url = f"{base_url}/{url}?miro-refresh"

        # Make request with proper headers
        headers = {"Authorization": f"Bearer {api_key}"}

        response = requests.get(jina_url, headers=headers, timeout=60)
        # debug_print(response)
        response.raise_for_status()

        # Get the content
        content = response.text.strip()
        print(content)


def main() -> None:
    # original
    # run_test(JINA_BASE_URL, JINA_API_KEY)
    # local test
    jina_url = f"{SERVICE_DOMAIN}/jina"
    run_test(jina_url, INTERNAL_KEY)
    # dokploy server test
    # run_test("https://cachev1.miromind.online/jina", INTERNAL_KEY)

if __name__ == "__main__":
    main()
