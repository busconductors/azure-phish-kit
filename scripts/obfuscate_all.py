#!/usr/bin/env python3
import re, random, os, glob

CAMPAIGN_DIR = '/Users/sk_hga/azure-phish-kit/campaign-emails'
OBFUSCATED_DIR = '/Users/sk_hga/azure-phish-kit/campaign-emails-obfuscated'

os.makedirs(f'{OBFUSCATED_DIR}/email', exist_ok=True)
os.makedirs(f'{OBFUSCATED_DIR}/attachments', exist_ok=True)

for subdir in ['email', 'attachments']:
    for f in sorted(glob.glob(f'{CAMPAIGN_DIR}/{subdir}/*.html')):
        name = os.path.basename(f)
        with open(f) as fh:
            html = fh.read()

        link_match = re.search(r'https://glnt\.cc/[^\x22\x27\s<>]+', html)
        phish_link = link_match.group(0) if link_match else ''
        if phish_link:
            chunks = [phish_link[i:i+20] for i in range(0, len(phish_link), 20)]
            chunk_str = ','.join(f"'{c}'" for c in chunks)
            js_decode = f"javascript:void((function(){{var p=[{chunk_str}];location.href=p.join('')}})())"
            html = html.replace(phish_link, js_decode)

        def entity_shuffle(text):
            r = []
            for c in text:
                if random.random() < 0.10 and c.isalpha():
                    r.append(f'&#{ord(c)};')
                else:
                    r.append(c)
            return ''.join(r)

        html = re.sub(r'>([^<]{4,})<', lambda m: '>' + entity_shuffle(m.group(1)) + '<', html)
        html = html.replace('javascript:void', 'jav&#97;script:void')

        out_path = f'{OBFUSCATED_DIR}/{subdir}/{name}'
        with open(out_path, 'w') as fh:
            fh.write(html)
        print(f'  {subdir}/{name}')

print('Done')
