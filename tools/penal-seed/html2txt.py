import re, sys, html, glob, os
from html.parser import HTMLParser

class P(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.out=[]; self.cur=[]; self.skip=0
        self.stack=[]  # list of bools: whether tag is strike
    def handle_starttag(self, tag, attrs):
        t=tag.lower()
        a=dict(attrs)
        style=(a.get('style') or '').lower().replace(' ','')
        is_strike = t in ('strike','s','del') or 'line-through' in style
        if t not in ('br','img','meta','link','input','hr'):
            self.stack.append((t,is_strike))
        if t in ('script','style'): self.skip+=1
        if t in ('p','div','br','tr','li','h1','h2','h3','h4','table','blockquote'):
            self.flush()
    def handle_endtag(self, tag):
        t=tag.lower()
        # pop until matching tag
        for i in range(len(self.stack)-1,-1,-1):
            if self.stack[i][0]==t:
                del self.stack[i:]
                break
        if t in ('script','style'): self.skip=max(0,self.skip-1)
        if t in ('p','div','tr','li','h1','h2','h3','h4','table','blockquote'):
            self.flush()
    def in_strike(self):
        return any(s for _,s in self.stack)
    def handle_data(self, data):
        if self.skip or self.in_strike(): return
        self.cur.append(data)
    def flush(self):
        txt=''.join(self.cur); self.cur=[]
        txt=txt.replace('\xa0',' ')
        txt=re.sub(r'\s+',' ',txt).strip()
        if txt: self.out.append(txt)

def decode(raw):
    if raw[:2]==b'\xff\xfe':
        raw=raw[:len(raw)-(len(raw)%2)]
        s=raw.decode('utf-16-le',errors='replace').lstrip('﻿')
        try:
            s=s.encode('latin-1').decode('utf-8')
        except Exception:
            pass
        return s
    m=re.search(rb'charset=([\w-]+)', raw[:4000], re.I)
    enc=(m.group(1).decode() if m else 'windows-1252')
    try: return raw.decode(enc)
    except Exception: return raw.decode('windows-1252',errors='replace')

def conv(src):
    s=decode(open(src,'rb').read())
    p=P(); p.feed(s); p.flush()
    return '\n'.join(p.out)

HERE = os.path.dirname(os.path.abspath(__file__))
for f in sorted(glob.glob(os.path.join(HERE, 'src', '*.html'))):
    txt=conv(f)
    o=f[:-5]+'.txt'
    open(o,'w').write(txt)
    print(o, len(txt.splitlines()))
