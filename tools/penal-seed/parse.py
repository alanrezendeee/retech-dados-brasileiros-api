"""Parser genérico para texto compilado do Planalto (uma linha por parágrafo).

Produz lista de artigos:
{
  num: 121, suf: 'A' | None, key: '121-A',
  rubrica: 'Homicídio simples' | None,
  caput: 'Matar alguém:', pena: 'reclusão, de seis a vinte anos.' | None,
  revogado: bool, vetado: bool,
  incisos: [{num:'I', texto, pena, alineas:[{letra, texto, pena}], revogado}],
  pars: [{num: 1|None(único), suf:'A'|None, key:'1-A'|'PU', rubrica, texto, pena, incisos:[...], revogado}],
  titulo, capitulo, parte, secao
}
"""
import re

ANNOT_WORDS = (r"Incluíd[oa]s?|Redação dada|Vide|Vigência|Regulamento|Promulgação|Renumerad[oa]|Acrescentad[oa]|"
               r"Alterad[oa]|Revogad[oa]|Suprimid[oa]|Expressão|Parágrafo|Dispositivo|Incluido|Redacao|Texto|"
               r"Vetad[oa]|Produção de efeitos|VETADO|Inciso|Alínea|Artigo|Caput|Vide|Nova redação|Numerad[oa]|"
               r"Restabelecid[oa]|Transformad[oa]|Mantid[oa]|Prorrogad[oa]|Observação|Ver|Vigencia|Regulamento|"
               r"Declarad[oa]|Deferid[oa]|ADI|ADPF|Medida Provisória|Decreto")
ANNOT_RE = re.compile(r"\s*\((?:" + ANNOT_WORDS + r")[^()]*\)", re.I)
# anotação com parêntese não fechado no fim da linha
ANNOT_OPEN_RE = re.compile(r"\s*\((?:" + ANNOT_WORDS + r")[^()]*$", re.I)
TRAIL_NOISE_RE = re.compile(r"\s*(?:\(Vigência\)|Vigência|Produção de efeitos|\(Regulamento\)|\(Vide\))\s*$", re.I)
BARE_ANNOT_RE = re.compile(r"\s*(?:Vide|Incluíd[oa]|Redação dada|Renumerad[oa]|Revogad[oa])\s+(?:pel[oa]\s+)?(?:Lei|Decreto-Lei|Medida Provisória|Decreto)\s+n[ºo°]?\s*[\d.]+(?:,?\s*de\s+[\d.]+(?:\s*de\s*\d{4})?)?\.?\s*$", re.I)

ART_RE = re.compile(r"^Art\.?\s*(\d+)\s*[ºo°]?\s*(?:-([A-Z](?:-[A-Z])?)(?![a-zà-úA-Z]))?\s*\.?\s*[ºo°]?\s*\.?\s*[-–\.]?\s*(.*)$")
PAR_RE = re.compile(r"^§\s*(\d+)\s*\.?\s*[ºo°]?\s*(?:-([A-Z](?:-[A-Z])?)(?![a-zà-úA-Z]))?\s*\.?\s*[-–]?\s*(.*)$")
PU_RE = re.compile(r"^Par[áa]grafo\s+[úu]nico\s*[\.\-–:]?\s*(.*)$", re.I)
INC_RE = re.compile(r"^([IVXL]+)(?:\s*-\s*([A-Z])(?=\s*[-–]))?\s*[-–\.]\s*(.*)$")
ALI_RE = re.compile(r"^([a-z])\)\s*(.*)$")
PENA_RE = re.compile(r"^Penas?\s*[-–:]?\s*(.*)$", re.I)
PENA_INLINE_RE = re.compile(r"\s*[:\.]?\s*\bPenas?\s*[-–:]?\s*(?=(?:[Dd]eten[çc][ãa]o|[Rr]eclus[ãa]o|[Pp]ris[ãa]o|[Mm]ulta))")
STRUCT_RE = re.compile(r"^(T[ÍI]TULO|CAP[ÍI]TULO|SE[ÇC][ÃA]O|SUBSE[ÇC][ÃA]O|LIVRO|PARTE|Se[çc][ãa]o|Subse[çc][ãa]o|Cap[íi]tulo|T[íi]tulo|Livro|Parte)\b")
STRUCT_NAME_RE = re.compile(r"^(Dos|Das|Do|Da|Disposi[çc][õo]es|Disposi[çc][ãa]o|De)\s", re.I)
END_RE = re.compile(r"^(Rio de Janeiro|Bras[íi]lia),\s+\d|^Este texto n[ãa]o substitui")

SMALL = {"a", "o", "e", "de", "da", "do", "das", "dos", "em", "contra", "à", "às", "ao", "aos", "por", "com",
         "sem", "sob", "ou", "no", "na", "nos", "nas", "para", "pelo", "pela", "que", "se", "um", "uma", "as", "os"}


def smart_title(s):
    words = s.lower().split()
    out = []
    for i, w in enumerate(words):
        if i > 0 and w in SMALL:
            out.append(w)
        else:
            out.append(w[:1].upper() + w[1:])
    return " ".join(out)


def strip_annot(s):
    prev = None
    while prev != s:
        prev = s
        s = ANNOT_RE.sub("", s)
    s = ANNOT_OPEN_RE.sub("", s)
    s = TRAIL_NOISE_RE.sub("", s)
    s = BARE_ANNOT_RE.sub("", s)
    s = s.replace("’", "").replace("‘", "").replace("”", "").replace("“", "")
    s = re.sub(r"\s+", " ", s).strip()
    s = re.sub(r"(\s*\.){2,}\s*$", ".", s)  # ". . . ." -> "."
    s = re.sub(r"\.{2,}", ".", s)
    s = re.sub(r";\s*\.$", ";", s)
    return s


def is_revoked_text(s):
    t = s.strip()
    return bool(re.match(r"^\(?\s*(Revogad[oa]|VETADO|Vetado)", t, re.I)) or t in ("", ".", ";", ";.")


def new_art(num, suf, rubrica, ctx):
    return dict(num=num, suf=suf, key=f"{num}-{suf}" if suf else str(num), rubrica=rubrica,
                caput="", pena=None, revogado=False, vetado=False, incisos=[], pars=[],
                titulo=ctx.get("titulo"), capitulo=ctx.get("capitulo"), parte=ctx.get("parte"), secao=ctx.get("secao"))


def split_inline_pena(obj, key):
    """Se o texto contém 'Pena - detenção...' inline, separa."""
    txt = obj.get(key) or ""
    m = PENA_INLINE_RE.search(txt)
    if m and m.start() > 0:
        before = txt[:m.start()].strip()
        after = txt[m.end():].strip()
        if after:
            obj[key] = before
            obj["pena"] = (obj.get("pena") + " " + after) if obj.get("pena") else after


def parse(text, start_at_art=1, prefix_fix=None):
    if prefix_fix:
        text = prefix_fix(text)
    lines = [l.strip() for l in text.splitlines()]
    arts = []
    art = None
    cur = None
    last_inc = None
    last_elem = None
    pending_rubrica = None
    ctx = {}
    started = False
    pending_struct = None

    for raw in lines:
        if not raw:
            continue
        if END_RE.match(raw):
            break
        line = raw
        m = ART_RE.match(line)
        if m:
            num = int(m.group(1)); suf = m.group(2); rest = m.group(3)
            if not started:
                if num != start_at_art:
                    continue
                started = True
            rest_clean = strip_annot(rest)
            a = new_art(num, suf, pending_rubrica, ctx)
            if is_revoked_text(rest) or is_revoked_text(rest_clean):
                a["revogado"] = True
                a["vetado"] = "VETADO" in rest.upper()
                a["caput"] = rest_clean.strip("“”\"") or "(Revogado)"
            else:
                a["caput"] = rest_clean
            dup = [i for i, x in enumerate(arts) if x["key"] == a["key"]]
            if dup:
                prev = arts[dup[0]]
                if a["revogado"] and not prev["revogado"]:
                    art = prev; cur = prev; last_inc = None; last_elem = ("art", prev)
                    pending_rubrica = None
                    continue
                arts[dup[0]] = a
            else:
                arts.append(a)
            art = a; cur = a; last_inc = None; last_elem = ("art", a)
            pending_rubrica = None
            continue
        if not started:
            # antes do primeiro artigo: ainda assim rastreia estrutura (PARTE/TÍTULO/CAPÍTULO) e rubrica
            if STRUCT_RE.match(line):
                key0 = line.split()[0].lower()
                key0 = re.sub(r"[íi]", "i", key0).replace("ç", "c").replace("ã", "a")
                if key0.startswith("parte"):
                    ctx["parte"] = smart_title(strip_annot(line)); ctx["titulo"] = None; ctx["capitulo"] = None; ctx["secao"] = None; pending_struct = None
                elif key0.startswith("titulo"):
                    ctx["titulo"] = strip_annot(line); ctx["capitulo"] = None; ctx["secao"] = None; pending_struct = "titulo"
                elif key0.startswith("capitulo"):
                    ctx["capitulo"] = strip_annot(line); ctx["secao"] = None; pending_struct = "capitulo"
                elif key0.startswith("secao") or key0.startswith("subsecao"):
                    ctx["secao"] = strip_annot(line); pending_struct = "secao"
                pending_rubrica = None
                continue
            c0 = strip_annot(line)
            if not c0:
                continue
            if c0.isupper() and len(c0) > 3:
                if pending_struct in ("titulo", "capitulo", "secao") and ctx.get(pending_struct):
                    base = ctx[pending_struct]
                    ctx[pending_struct] = (base + " – " + smart_title(c0)) if "–" not in base else (base + " " + smart_title(c0))
                pending_rubrica = None
                continue
            if 3 <= len(c0) <= 140 and not c0.endswith((":", ";", ".")) and not c0.startswith("("):
                pending_rubrica = c0
            else:
                pending_rubrica = None
            continue
        if STRUCT_RE.match(line):
            key = line.split()[0].lower()
            key = re.sub(r"[íi]", "i", key).replace("ç", "c").replace("ã", "a")
            if key.startswith("titulo"):
                ctx["titulo"] = strip_annot(line); ctx["capitulo"] = None; ctx["secao"] = None
                pending_struct = "titulo"
            elif key.startswith("capitulo"):
                ctx["capitulo"] = strip_annot(line); ctx["secao"] = None
                pending_struct = "capitulo"
            elif key.startswith("secao") or key.startswith("subsecao"):
                ctx["secao"] = strip_annot(line)
                pending_struct = "secao"
            elif key.startswith("parte"):
                ctx["parte"] = smart_title(strip_annot(line))
                ctx["titulo"] = None; ctx["capitulo"] = None; ctx["secao"] = None
                pending_struct = None
            else:
                pending_struct = None
            pending_rubrica = None
            continue
        clean = strip_annot(line)
        if not clean:
            continue
        is_caps = clean.isupper() and len(clean) > 3 and not PENA_RE.match(clean)
        is_struct_name = bool(STRUCT_NAME_RE.match(clean)) and len(clean) <= 120 and not clean.endswith((":", ";"))
        if is_caps or (is_struct_name and not (clean.endswith(".") and len(clean) > 60)):
            if pending_struct in ("titulo", "capitulo", "secao") and ctx.get(pending_struct):
                base = ctx[pending_struct]
                ctx[pending_struct] = (base + " – " + smart_title(clean)) if "–" not in base else (base + " " + smart_title(clean))
            pending_rubrica = None
            continue
        pending_struct = None

        m = PAR_RE.match(line)
        mpu = PU_RE.match(line) if not m else None
        if m or mpu:
            if m:
                pnum = int(m.group(1)); psuf = m.group(2); rest = m.group(3)
                pkey = f"{pnum}-{psuf}" if psuf else str(pnum)
            else:
                pnum = None; psuf = None; rest = mpu.group(1); pkey = "PU"
            rest = re.sub(r"^[ºo°]\s+(?=[A-ZÀ-Ú(])", "", rest)
            rest_clean = strip_annot(rest)
            p = dict(num=pnum, suf=psuf, key=pkey, rubrica=pending_rubrica, texto=rest_clean, pena=None,
                     incisos=[], revogado=False)
            if is_revoked_text(rest) or is_revoked_text(rest_clean):
                p["revogado"] = True
                p["texto"] = rest_clean or "(Revogado)"
            if art is None:
                continue
            dup = [i for i, x in enumerate(art["pars"]) if x["key"] == pkey]
            if dup:
                if not p["revogado"]:
                    art["pars"][dup[0]] = p
                else:
                    pending_rubrica = None
                    continue
            else:
                art["pars"].append(p)
            split_inline_pena(p, "texto")
            cur = p; last_inc = None; last_elem = ("par", p)
            pending_rubrica = None
            continue
        m = INC_RE.match(line)
        if m and cur is not None:
            rest = m.group(3)
            rest_clean = strip_annot(rest)
            inum = m.group(1) + (("-" + m.group(2)) if m.group(2) else "")
            inc = dict(num=inum, texto=rest_clean, pena=None, alineas=[], revogado=False, rubrica=pending_rubrica)
            if is_revoked_text(rest) or is_revoked_text(rest_clean):
                inc["revogado"] = True
                inc["texto"] = rest_clean or "(Revogado)"
            dup = [i for i, x in enumerate(cur["incisos"]) if x["num"] == inc["num"]]
            if dup:
                if not inc["revogado"]:
                    cur["incisos"][dup[0]] = inc
            else:
                cur["incisos"].append(inc)
            split_inline_pena(inc, "texto")
            last_inc = inc; last_elem = ("inc", inc)
            pending_rubrica = None
            continue
        m = ALI_RE.match(line)
        if m and last_inc is not None:
            rest_clean = strip_annot(m.group(2))
            ali = dict(letra=m.group(1), texto=rest_clean, pena=None, revogado=is_revoked_text(m.group(2)) or is_revoked_text(rest_clean))
            last_inc["alineas"].append(ali)
            last_elem = ("ali", ali)
            pending_rubrica = None
            continue
        m = PENA_RE.match(clean)
        if m and last_elem is not None and re.match(r"^Penas?\s*[-–:]", clean, re.I):
            kind, obj = last_elem
            ptxt = m.group(1).strip()
            if kind in ("inc", "ali"):
                target = cur
                if target.get("pena"):
                    if re.sub(r"\W", "", target["pena"]).lower() == re.sub(r"\W", "", ptxt).lower():
                        pass
                    else:
                        obj["pena"] = ptxt
                else:
                    target["pena"] = ptxt
            else:
                if obj.get("pena"):
                    if re.sub(r"\W", "", obj["pena"]).lower() != re.sub(r"\W", "", ptxt).lower():
                        obj["pena"] = obj["pena"] + " " + ptxt
                else:
                    obj["pena"] = ptxt
            pending_rubrica = None
            continue
        if clean.startswith(("Infração", "Penalidade", "Medida administrativa")):
            if last_elem is not None:
                kind, obj = last_elem
                tgt_key = "caput" if kind == "art" else "texto"
                obj[tgt_key] = ((obj.get(tgt_key) or "") + " " + clean).strip()
            continue
        if len(clean) <= 140 and 3 <= len(clean) and not clean.endswith((":", ";", ".")) and not clean.startswith("("):
            pending_rubrica = clean.strip("“”\"")
            continue
        if last_elem is not None:
            kind, obj = last_elem
            tgt_key = "caput" if kind == "art" else "texto"
            obj[tgt_key] = ((obj.get(tgt_key) or "") + " " + clean).strip()
            split_inline_pena(obj, tgt_key)
        pending_rubrica = None
    for a in arts:
        split_inline_pena(a, "caput")
    return arts


if __name__ == "__main__":
    import sys
    arts = parse(open(sys.argv[1]).read(), int(sys.argv[2]) if len(sys.argv) > 2 else 1)
    want = set(sys.argv[3:])
    for a in arts:
        if want and a["key"] not in want:
            continue
        print(f"=== Art. {a['key']} | rubrica={a['rubrica']} | rev={a['revogado']} | {a['parte']} | {a['titulo']} | {a['capitulo']} | {a['secao']}")
        print("  caput:", a["caput"][:300])
        print("  pena:", a["pena"])
        for i in a["incisos"]:
            print(f"    {i['num']} - {i['texto'][:120]} | pena={i['pena']} | rev={i['revogado']}")
            for al in i["alineas"]:
                print(f"       {al['letra']}) {al['texto'][:100]}")
        for p in a["pars"]:
            print(f"  § {p['key']} | rubrica={p['rubrica']} | rev={p['revogado']} | pena={p['pena']}")
            print("     ", p["texto"][:200])
            for i in p["incisos"]:
                print(f"      {i['num']} - {i['texto'][:120]} | pena={i['pena']} | rev={i['revogado']}")
                for al in i["alineas"]:
                    print(f"         {al['letra']}) {al['texto'][:100]}")
    print("TOTAL arts:", len(arts))
