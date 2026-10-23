"""Build the two static policy pages from reviewed Markdown; no third-party deps."""
from pathlib import Path
import html
import re

root = Path(__file__).resolve().parents[1]
output = root / 'site/dist'

def inline(text):
    text = html.escape(text)
    text = re.sub(r'`([^`]+)`', r'<code>\1</code>', text)
    text = re.sub(r'\[([^\]]+)\]\((https://[^\s)]+)\)', r'<a href="\2">\1</a>', text)
    return text.replace('contacticev1@gmail.com', '<!--email_off-->contacticev1@gmail.com<!--/email_off-->')

def markdown(source):
    blocks, paragraph = [], []
    def flush():
        if paragraph:
            blocks.append('<p>' + inline(' '.join(paragraph)) + '</p>')
            paragraph.clear()
    for line in source.splitlines():
        if not line.strip():
            flush()
        elif line.startswith('## '):
            flush()
            blocks.append('<h2>' + inline(line[3:]) + '</h2>')
        elif line.startswith('# '):
            flush()
            blocks.append('<h1>' + inline(line[2:]) + '</h1>')
        else:
            paragraph.append(line)
    flush()
    return '\n'.join(blocks)

for filename, route, title in [('PRIVACY.md', 'privacy', 'Privacy policy'), ('TERMS.md', 'terms', 'Usage and limitations')]:
    body = markdown((root / 'docs/branding' / filename).read_text(encoding='utf-8'))
    page = f'''<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>{title} — HomeVault Personal</title><link rel="canonical" href="https://homevault.suwiwat.com/{route}"><link rel="stylesheet" href="/styles.css"></head><body><a class="skip" href="#main">Skip to content</a><header><a class="brand" href="/">HomeVault<span>.</span></a><nav aria-label="Main navigation"><a href="/">Home</a><a href="/privacy">Privacy</a><a href="/terms">Usage</a></nav></header><main class="legal" id="main">{body}</main><footer><span>HomeVault Personal</span><div><a href="/">Home</a><!--email_off--><a href="mailto:contacticev1@gmail.com">Contact</a><!--/email_off--></div></footer></body></html>'''
    (output / (route + '.html')).write_text(page, encoding='utf-8')
print('Built HomeVault policy pages; no credentials or runtime data included.')
