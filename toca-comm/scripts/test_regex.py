import re
from update_templates import TEMPLATES, BASE_HTML
t = TEMPLATES[2]
html_content = BASE_HTML.replace("{{TITLE}}", t["title"])
html_content = html_content.replace("{{BODY_CONTENT}}", t["body"])
html_content = html_content.replace("{{ACTION_URL}}", t["action_url"])
html_content = html_content.replace("{{ACTION_TEXT}}", t["action_text"])
print(re.findall(r"\{\{.*?\}\}", html_content))
