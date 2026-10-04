# -*- coding: utf-8 -*-
"""Gera seeds/penal.json a partir dos textos compilados do Planalto."""
import hashlib
import json
import os
import re
import sys
from collections import Counter, OrderedDict

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.abspath(os.path.join(HERE, "..", ".."))
SRC = os.path.join(HERE, "src")
OUT = os.path.join(ROOT, "seeds", "penal.json")
sys.path.insert(0, HERE)
from parse import parse, smart_title  # noqa
from overrides import DESC, TIPO, PENA  # noqa

DATA_ATUALIZACAO = "outubro/2026"


def rng(spec):
    out = set()
    for part in spec.split(","):
        part = part.strip()
        if "-" in part:
            a, b = part.split("-")
            out.update(range(int(a), int(b) + 1))
        else:
            out.add(int(part))
    return out


def fix_prefeitos(t):
    return t.replace("\nIl - ", "\nII - ").replace("; Il - ", ";\nII - ")


# (file, code, legislacao, legislacaoNome, fonte, artigos, tipoDefault, prefix_fix)
LAWS = [
    ("cp_planalto", "CP", "CP", "Código Penal",
     "https://www.planalto.gov.br/ccivil_03/decreto-lei/del2848compilado.htm", "1-361", "crime", None),
    ("lcp", "LCP", "LCP", "Lei de Contravenções Penais",
     "https://www.planalto.gov.br/ccivil_03/decreto-lei/del3688.htm", "18-70", "contravencao", None),
    ("drg", "DRG", "Lei 11.343/2006", "Lei de Drogas",
     "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2006/lei/l11343.htm", "28,33-41", "crime", None),
    ("des", "DES", "Lei 10.826/2003", "Estatuto do Desarmamento",
     "https://www.planalto.gov.br/ccivil_03/leis/2003/l10.826.htm", "12-20", "crime", None),
    ("mdp", "MDP", "Lei 11.340/2006", "Lei Maria da Penha",
     "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2006/lei/l11340.htm", "5,7,24", "crime", None),
    ("henry", "HBO", "Lei 14.344/2022", "Lei Henry Borel",
     "https://www.planalto.gov.br/ccivil_03/_ato2019-2022/2022/lei/l14344.htm", "25", "crime", None),
    ("eca", "ECA", "ECA", "Estatuto da Criança e do Adolescente",
     "https://www.planalto.gov.br/ccivil_03/leis/l8069.htm", "225-244", "crime", None),
    ("idoso", "IDO", "Lei 10.741/2003", "Estatuto da Pessoa Idosa",
     "https://www.planalto.gov.br/ccivil_03/leis/2003/l10.741.htm", "96-108", "crime", None),
    ("pcd", "PCD", "Lei 13.146/2015", "Estatuto da Pessoa com Deficiência",
     "https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2015/lei/l13146.htm", "88-91", "crime", None),
    ("l7853", "DEF", "Lei 7.853/1989", "Lei de Apoio às Pessoas com Deficiência",
     "https://www.planalto.gov.br/ccivil_03/leis/l7853.htm", "8", "crime", None),
    ("ctb", "CTB", "CTB", "Código de Trânsito Brasileiro",
     "https://www.planalto.gov.br/ccivil_03/leis/l9503compilado.htm", "165,302-312", "crime", None),
    ("amb", "AMB", "Lei 9.605/98", "Lei de Crimes Ambientais",
     "https://www.planalto.gov.br/ccivil_03/leis/l9605.htm", "29-69", "crime", None),
    ("cdc", "CDC", "CDC", "Código de Defesa do Consumidor",
     "https://www.planalto.gov.br/ccivil_03/leis/l8078compilado.htm", "61-80", "crime", None),
    ("ecopop", "EPO", "Lei 1.521/1951", "Lei dos Crimes contra a Economia Popular",
     "https://www.planalto.gov.br/ccivil_03/leis/l1521.htm", "2-4", "crime", None),
    ("l8137", "OTE", "Lei 8.137/1990", "Lei dos Crimes contra a Ordem Tributária, Econômica e Relações de Consumo",
     "https://www.planalto.gov.br/ccivil_03/leis/l8137.htm", "1-12", "crime", None),
    ("lvd", "LVD", "Lei 9.613/98", "Lei de Lavagem de Dinheiro",
     "https://www.planalto.gov.br/ccivil_03/leis/l9613.htm", "1", "crime", None),
    ("sfn", "SFN", "Lei 7.492/1986", "Lei dos Crimes contra o Sistema Financeiro Nacional",
     "https://www.planalto.gov.br/ccivil_03/leis/l7492.htm", "2-23", "crime", None),
    ("cvm", "CVM", "Lei 6.385/1976", "Lei do Mercado de Valores Mobiliários",
     "https://www.planalto.gov.br/ccivil_03/leis/l6385.htm", "27", "crime", None),
    ("falim", "FAL", "Lei 11.101/2005", "Lei de Falências e Recuperação de Empresas",
     "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2005/lei/l11101.htm", "168-178", "crime", None),
    ("propind", "PIN", "Lei 9.279/1996", "Lei de Propriedade Industrial",
     "https://www.planalto.gov.br/ccivil_03/leis/l9279.htm", "183-195", "crime", None),
    ("soft", "SOF", "Lei 9.609/1998", "Lei do Software",
     "https://www.planalto.gov.br/ccivil_03/leis/l9609.htm", "12", "crime", None),
    ("tort", "TOR", "Lei 9.455/1997", "Lei de Tortura",
     "https://www.planalto.gov.br/ccivil_03/leis/l9455.htm", "1-2", "crime", None),
    ("rac", "RAC", "Lei 7.716/1989", "Lei do Racismo",
     "https://www.planalto.gov.br/ccivil_03/leis/l7716.htm", "1-20", "crime", None),
    ("hiv", "HIV", "Lei 12.984/2014", "Lei de Discriminação de Portadores do HIV",
     "https://www.planalto.gov.br/ccivil_03/_ato2011-2014/2014/lei/l12984.htm", "1", "crime", None),
    ("l9029", "DIS", "Lei 9.029/1995", "Lei de Práticas Discriminatórias no Trabalho",
     "https://www.planalto.gov.br/ccivil_03/leis/l9029.htm", "2", "crime", None),
    ("orcrim", "ORC", "Lei 12.850/2013", "Lei de Organização Criminosa",
     "https://www.planalto.gov.br/ccivil_03/_ato2011-2014/2013/lei/l12850.htm", "1-2,18-21", "crime", None),
    ("hed", "HED", "Lei 8.072/1990", "Lei dos Crimes Hediondos",
     "https://www.planalto.gov.br/ccivil_03/leis/l8072.htm", "1-2", "crime", None),
    ("terror", "TER", "Lei 13.260/2016", "Lei Antiterrorismo",
     "https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2016/lei/l13260.htm", "2-7", "crime", None),
    ("geno", "GEN", "Lei 2.889/1956", "Lei do Genocídio",
     "https://www.planalto.gov.br/ccivil_03/leis/l2889.htm", "1-3", "crime", None),
    ("abuso", "ABU", "Lei 13.869/2019", "Lei de Abuso de Autoridade",
     "https://www.planalto.gov.br/ccivil_03/_ato2019-2022/2019/lei/l13869.htm", "9-38", "crime", None),
    ("intercep", "INT", "Lei 9.296/1996", "Lei de Interceptação Telefônica",
     "https://www.planalto.gov.br/ccivil_03/leis/l9296.htm", "10", "crime", None),
    ("prefeitos", "PRE", "Decreto-Lei 201/1967", "Lei dos Crimes de Responsabilidade de Prefeitos",
     "https://www.planalto.gov.br/ccivil_03/decreto-lei/del0201.htm", "1", "crime", fix_prefeitos),
    ("eleitoral", "ELE", "Lei 4.737/1965", "Código Eleitoral",
     "https://www.planalto.gov.br/ccivil_03/leis/l4737compilado.htm", "289-354", "crime", None),
    ("transp", "TRA", "Lei 9.434/1997", "Lei de Transplantes",
     "https://www.planalto.gov.br/ccivil_03/leis/l9434.htm", "14-20", "crime", None),
    ("bio", "BIO", "Lei 11.105/2005", "Lei de Biossegurança",
     "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2005/lei/l11105.htm", "24-29", "crime", None),
    ("solo", "SOL", "Lei 6.766/1979", "Lei do Parcelamento do Solo Urbano",
     "https://www.planalto.gov.br/ccivil_03/leis/l6766.htm", "50-52", "crime", None),
]

# Artigos específicos a excluir (não penais) dentro das faixas
EXCLUDE = {"CTB:165-A", "CTB:165-B", "CTB:165-C", "CTB:165-D", "CVM:28", "CVM:31", "CVM:32", "CVM:27-A", "CVM:27-B",
           "MDP:24"}

# ----------------------------------------------------------------------------
# Normalização de texto / pena
# ----------------------------------------------------------------------------
ORTO = {
    "alguem": "alguém", "futil": "fútil", "impossivel": "impossível", "constitue": "constitui",
    "constituem": "constituem", "dêle": "dele", "dêste": "deste", "nêste": "neste", "êle": "ele", "êste": "este",
    "êsse": "esse", "aquêle": "aquele", "aquêles": "aqueles", "govêrno": "governo", "emprêsa": "empresa",
    "idéia": "ideia", "vôo": "voo", "assembléia": "assembleia", "jóia": "joia", "pôr": "pôr", "sôbre": "sobre",
    "quís": "quis", "três": "três", "môvel": "móvel", "alem": "além", "pontual": "pontual",
    "nêle": "nele", "dêles": "deles", "êles": "eles", "fôr": "for", "pêso": "peso", "indispensavel": "indispensável",
    "cadaver": "cadáver", "indivíduos": "indivíduos", "numero": "número", "tôda": "toda", "tôdas": "todas",
    "sòmente": "somente", "cêdo": "cedo", "dêsse": "desse", "nêste": "neste", "açambarcar": "açambarcar",
}
ORTO_RE = re.compile(r"\b(" + "|".join(re.escape(k) for k in ORTO) + r")\b")


def norm_text(s):
    if not s:
        return s
    s = s.replace("qü", "qu").replace("gü", "gu").replace("Qü", "Qu")
    s = ORTO_RE.sub(lambda m: ORTO[m.group(1)], s)
    s = re.sub(r"\s+", " ", s).strip()
    s = re.sub(r"\s+([;:,.])", r"\1", s)
    return s


NUM_WORDS = {
    "um": 1, "uma": 1, "dois": 2, "duas": 2, "três": 3, "tres": 3, "quatro": 4, "cinco": 5, "seis": 6, "sete": 7,
    "oito": 8, "nove": 9, "dez": 10, "onze": 11, "doze": 12, "treze": 13, "quatorze": 14, "catorze": 14,
    "quinze": 15, "dezesseis": 16, "dezessete": 17, "dezoito": 18, "dezenove": 19, "vinte": 20, "trinta": 30,
    "quarenta": 40, "cinquenta": 50, "cinqüenta": 50, "sessenta": 60,
}


def word2num(w):
    w = w.strip().lower()
    if w.isdigit():
        return int(w)
    if w in NUM_WORDS:
        return NUM_WORDS[w]
    m = re.match(r"^(vinte|trinta|quarenta|cinquenta|cinqüenta|sessenta) e (\w+)$", w)
    if m and m.group(2) in NUM_WORDS:
        return NUM_WORDS[m.group(1)] + NUM_WORDS[m.group(2)]
    return None


def norm_pena_text(p):
    p = p.strip().rstrip(".").strip()
    p = re.sub(r"\s*\(\s*[a-zçáéíóúãõâêôü\s]+\)", "", p)  # (um), (dois), (mil e quinhentos)
    p = p.replace("qü", "qu")

    def conv(m):
        a, ua, b, unit = m.group(1), m.group(2), m.group(3), m.group(4)
        na, nb = word2num(a), word2num(b)
        if na is None or nb is None:
            return m.group(0)
        if ua:
            return f"de {na} {ua} a {nb} {unit}"
        return f"de {na} a {nb} {unit}"

    p = re.sub(r"\bde (\w+(?: e \w+)?) (?:(anos?|meses|mês|dias?) )?a (\w+(?: e \w+)?) (anos?|meses|mês|dias?)", conv, p)

    def conv2(m):
        n = word2num(m.group(1))
        return f"até {n} {m.group(2)}" if n is not None else m.group(0)

    p = re.sub(r"\bat[ée] (\w+(?: e \w+)?) (anos?|meses|mês|dias?)", conv2, p)
    p = re.sub(r"\s+", " ", p).strip(" ,")
    return p


def split_pena(p):
    """'reclusão, de seis a vinte anos, e multa' -> ('Reclusão, de 6 a 20 anos', 'e multa')"""
    if not p:
        return "", ""
    p = norm_pena_text(p)
    m = re.match(r"^(reclus[ãa]o|deten[çc][ãa]o|pris[ãa]o simples|multa)[, ]*\s*"
                 r"((?:de\s+\S+(?:\s+(?:anos?|meses|mês|dias?))?\s+a\s+\S+\s+(?:anos?|meses|mês|dias?))|(?:at[ée]\s+\S+\s+(?:anos?|meses|mês|dias?)))(.*)$",
                 p, re.I)
    if m:
        tipo = m.group(1).lower()
        tipo = {"reclusao": "reclusão", "detencao": "detenção"}.get(tipo, tipo)
        tipo = tipo[:1].upper() + tipo[1:]
        pmin = f"{tipo}, {m.group(2).strip()}"
        rest = m.group(3).strip(" ,;")
        return pmin, rest
    return p[:1].upper() + p[1:], ""


PENA_INLINE = re.compile(
    r"(?:a pena (?:é|será|passa a ser|é a) de|pena de|pun[íi]vel com|punid[oa]s? com|incorre(?:m|rá|rão)? na(?:s)? pena(?:s)? de|"
    r"sujeit[oa]s? [àa] pena de|aplica-se a pena de|a reclus[ãa]o é de|é pena de|ser[áa] de|será a de)\s+"
    r"((?:reclus[ãa]o|deten[çc][ãa]o|pris[ãa]o simples)[^;]*?(?:anos?|meses|mês|dias?)"
    r"(?:\s+a\s+[^\s,;]+(?:\s+\([^)]*\))?\s+(?:anos?|meses|mês|dias?))?(?:,? e multa|,? ou multa)?)",
    re.I)


def pena_from_text(t):
    m = PENA_INLINE.search(t or "")
    if m:
        return m.group(1)
    return None


# ----------------------------------------------------------------------------
# Descrições
# ----------------------------------------------------------------------------
GENERIC_RUBRICAS = {
    "aumento de pena", "diminuição de pena", "causa de diminuição de pena", "caso de diminuição de pena",
    "modalidade culposa", "forma qualificada", "formas qualificadas", "substituição da pena", "exceção da verdade",
    "ação penal", "coautoria", "pena", "exclusão do crime", "exclusão de ilicitude", "retratação",
    "concurso de pessoas", "aumento da pena", "redução ou substituição da pena", "pena de tentativa",
    "prática do crime com o fim de lucro", "figura privilegiada",
}


def short(t, n=90):
    t = (t or "").strip().rstrip(";:.,")
    t = re.sub(r"^\(?(VETADO|Revogado)\)?\.?$", "", t)
    if len(t) <= n:
        return t
    cut = t[:n]
    if " " in cut:
        cut = cut[:cut.rfind(" ")]
    return cut + "…"


def fallback_desc(caput):
    t = (caput or "").strip()
    t = re.sub(r"[:;.,]\s*$", "", t)
    # primeira oração até ':' ou ';'
    t = re.split(r"[:;]", t)[0]
    return short(t, 90)


def par_label(p):
    if p["key"] == "PU":
        return "parágrafo único"
    return "§ " + ordinal(p["num"]) + (f"-{p['suf']}" if p.get("suf") else "")


def ordinal(n):
    return f"{n}º" if n < 10 else str(n)


def desc_paragrafo(base, p, has_pena):
    rub = (p.get("rubrica") or "").strip()
    t = p.get("texto") or ""
    if rub and len(rub) >= 4:
        if rub.lower() in GENERIC_RUBRICAS:
            return f"{base} – {rub.lower()}"
        return rub
    tl = t.lower()
    if re.match(r"^(nas? mesmas? penas?|incorre|incide|est[áa] sujeito|aplica-se a mesma pena|sujeita-se|incorrer[áã])", tl):
        return f"{base} – figura equiparada"
    if re.search(r"(perd[ãa]o judicial|deixar de aplicar a pena|isento de pena|isenta de pena)", tl):
        return f"{base} – perdão judicial / isenção de pena"
    if re.search(r"(aumenta|aumentad|em dobro|duplicad|triplo|em triplo|elevad)", tl) and not re.search(r"(reduzid|diminu)", tl):
        return f"{base} – causa de aumento de pena"
    if re.search(r"(reduzid|diminu[íi]d|diminui|reduzir a pena|redu[çc][ãa]o)", tl):
        return f"{base} – causa de diminuição de pena"
    if re.search(r"culpos", tl):
        return f"{base} – modalidade culposa"
    if re.search(r"resulta(?:r)? (?:a )?morte|resulta(?:r)? em morte", tl) and tl.startswith("se"):
        return f"{base} – resultado morte"
    if re.search(r"les[ãa]o corporal (?:de natureza )?grav", tl) and tl.startswith("se"):
        return f"{base} – resultado lesão corporal grave"
    if has_pena and tl.startswith("se "):
        return f"{base} – forma qualificada"
    if re.search(r"(somente se procede|procede-se mediante|a[çc][ãa]o penal|mediante representa[çc][ãa]o|mediante queixa)", tl):
        return f"{base} – ação penal"
    if re.search(r"(n[ãa]o constitui crime|n[ãa]o se aplica|n[ãa]o [ée] pun[íi]vel|n[ãa]o se compreende|n[ãa]o h[áa] crime|exclui)", tl):
        return f"{base} – exclusão / inaplicabilidade"
    if re.search(r"(equipara|considera-se|consideram-se|para os efeitos|entende-se por|compreende)", tl):
        return f"{base} – definição / equiparação"
    if has_pena:
        return f"{base} – forma qualificada"
    return f"{base} – {par_label(p)}"


# ----------------------------------------------------------------------------
# Construção das entradas
# ----------------------------------------------------------------------------
def sha(leg, codigo, texto):
    return hashlib.sha256(f"{leg}:{codigo}:{texto}".encode("utf-8")).hexdigest()


def fmt_art(key):
    return f"Art. {key}"


def leg_suffix(code, legislacao):
    fem = {"LCP"}
    if legislacao.startswith("Lei "):
        return "da " + legislacao
    if legislacao.startswith("Decreto-Lei"):
        return "do " + legislacao
    return ("da " if legislacao in fem else "do ") + legislacao


entries = []
ordem = [0]


def add(entry):
    ordem[0] += 1
    entry["ordem"] = ordem[0]
    entries.append(entry)


def make_entry(law, codigo, artigo_num, paragrafo, inciso, alinea, descricao, texto, tipo, pena_min, pena_max,
               codigo_fmt, nivel, art):
    _, code, legislacao, nome, fonte, _, _, _ = law
    texto = norm_text(texto)
    e = OrderedDict()
    e["codigo"] = codigo
    e["artigo"] = artigo_num
    e["paragrafo"] = paragrafo
    e["inciso"] = inciso
    e["alinea"] = alinea
    e["descricao"] = norm_text(descricao)
    e["textoCompleto"] = texto
    e["tipo"] = tipo
    e["nivel"] = nivel
    e["parte"] = art.get("parte") or ""
    e["titulo"] = clean_struct(art.get("titulo"))
    e["capitulo"] = clean_struct(art.get("capitulo"))
    e["legislacao"] = legislacao
    e["legislacaoNome"] = nome
    e["penaMin"] = pena_min or ""
    e["penaMax"] = pena_max or ""
    e["codigoFormatado"] = f"{codigo_fmt} {leg_suffix(code, legislacao)}"
    e["fonte"] = fonte
    e["dataAtualizacao"] = DATA_ATUALIZACAO
    e["hashConteudo"] = sha(legislacao, codigo, texto)
    e["idUnico"] = f"{code}:{codigo}"
    return e


def clean_struct(s):
    if not s:
        return ""
    s = re.sub(r"\s+", " ", s).strip()
    s = re.sub(r"^(T[ÍI]TULO|CAP[ÍI]TULO|SE[ÇC][ÃA]O)\s+", lambda m: smart_title(m.group(1)) + " ", s)
    s = re.sub(r"^(Título|Capítulo|Seção)\s+([IVXLC]+(?:-[A-Z])?)\b", lambda m: f"{m.group(1)} {m.group(2).upper()}", s)
    return s


def effective_pena(obj, inherit=None):
    """Retorna (penaMin, penaMax, has_own)"""
    if obj.get("pena"):
        a, b = split_pena(obj["pena"])
        return a, b, True
    txt = obj.get("texto") if "texto" in obj else obj.get("caput")
    inline = pena_from_text(txt)
    if inline:
        a, b = split_pena(inline)
        return a, b, True
    if inherit:
        return inherit[0], inherit[1], False
    return "", "", False


INHERIT_RE = re.compile(r"^(nas? mesmas? penas?|incorre|incide|est[áa] sujeito|aplica-se a mesma pena|sujeita-se|incorrer[áã]|incorrem)", re.I)


def build_law(law):
    fname, code, legislacao, nome, fonte, spec, tipo_default, pfix = law
    text = open(os.path.join(SRC, f"{fname}.txt"), encoding="utf-8").read()
    arts = parse(text, 1, pfix)
    want = rng(spec)
    for art in arts:
        if art["num"] not in want:
            continue
        idu = f"{code}:{art['key']}"
        if idu in EXCLUDE:
            continue
        # ---- tipo do artigo
        any_pena = bool(art["pena"]) or bool(pena_from_text(art["caput"])) or any(p.get("pena") for p in art["pars"]) \
            or any(i.get("pena") for i in art["incisos"]) or any(pena_from_text(p.get("texto")) for p in art["pars"])
        if art["revogado"]:
            tipo = "revogado"
        elif idu in TIPO:
            tipo = TIPO[idu]
        elif any_pena:
            tipo = tipo_default
        else:
            tipo = "disposicao"
        # ---- descrição do artigo
        if art["revogado"]:
            desc = f"Art. {art['key']} – revogado"
        else:
            desc = DESC.get(idu) or (art["rubrica"] if art["rubrica"] and len(art["rubrica"]) >= 3 else None) or fallback_desc(art["caput"])
            desc = desc.strip().rstrip(".")
        # ---- pena do artigo
        if idu in PENA:
            pmin, pmax = PENA[idu]
        else:
            pmin, pmax, _ = effective_pena(art)
        art_pena = (pmin, pmax)
        # ---- texto do caput (+ incisos do caput inline)
        caput = art["caput"]
        if art["revogado"]:
            caput = art["caput"] if art["caput"] and art["caput"] != "." else "(Revogado)"
            if not caput.lower().startswith("(revogado") and "revogado" not in caput.lower() and "vetado" not in caput.lower():
                caput = "(Revogado)"
        inc_txt = " ".join(f"{i['num']} - {i['texto']}" + ("".join(f" {a['letra']}) {a['texto']}" for a in i["alineas"] if not a["revogado"] and a["texto"].strip(" ;.")) if i["alineas"] else "")
                           for i in art["incisos"] if not i["revogado"] and i["texto"].strip(" ;."))
        texto_art = (caput + " " + inc_txt).strip() if inc_txt else caput
        base_entry = make_entry(law, art["key"], art["num"], None, None, None, desc, texto_art, tipo, pmin, pmax,
                                fmt_art(art["key"]), "artigo", art)
        add(base_entry)
        if art["revogado"]:
            continue
        # ---- incisos do caput
        emit_incisos(law, art, art, art["key"], fmt_art(art["key"]), art["num"], None, desc, tipo, art_pena)
        # ---- parágrafos
        for p in art["pars"]:
            if p["revogado"]:
                continue
            if not p["texto"].strip(" ;.") and not p["incisos"]:
                continue
            pnum = 1 if p["key"] == "PU" else p["num"]
            pcode = f"{art['key']}.{pnum}" + (f"-{p['suf']}" if p.get("suf") else "")
            pidu = f"{code}:{pcode}"
            pfmt = f"{fmt_art(art['key'])}, {par_label(p)}"
            own = bool(p.get("pena")) or bool(pena_from_text(p.get("texto")))
            if pidu in PENA:
                ppmin, ppmax = PENA[pidu]
            else:
                inherit = art_pena if INHERIT_RE.match(p["texto"] or "") else None
                ppmin, ppmax, _ = effective_pena(p, inherit)
            pdesc = DESC.get(pidu) or desc_paragrafo(desc, p, own)
            pinc = " ".join(f"{i['num']} - {i['texto']}" + ("".join(f" {a['letra']}) {a['texto']}" for a in i["alineas"] if not a["revogado"] and a["texto"].strip(" ;.")) if i["alineas"] else "")
                            for i in p["incisos"] if not i["revogado"] and i["texto"].strip(" ;."))
            ptexto = (p["texto"] + " " + pinc).strip() if pinc else p["texto"]
            ptipo = tipo
            add(make_entry(law, pcode, art["num"], pnum, None, None, pdesc, ptexto, ptipo, ppmin, ppmax, pfmt, "paragrafo", art))
            emit_incisos(law, art, p, pcode, pfmt, art["num"], pnum, pdesc, ptipo, (ppmin, ppmax))


def emit_incisos(law, art, container, parent_code, parent_fmt, artigo_num, pnum, parent_desc, tipo, parent_pena):
    code = law[1]
    for i in container["incisos"]:
        if i["revogado"] or not i["texto"].strip(" ;.") or i["texto"].strip() in ("(Revogado)", "(VETADO)"):
            continue
        icode = f"{parent_code}.{i['num']}"
        iidu = f"{code}:{icode}"
        ifmt = f"{parent_fmt}, {i['num']}"
        if iidu in PENA:
            ipmin, ipmax = PENA[iidu]
        else:
            ipmin, ipmax, _ = effective_pena(i, parent_pena)
        idesc = DESC.get(iidu) or f"{parent_desc} – {short(i['texto'], 80)}"
        ali_txt = " ".join(f"{a['letra']}) {a['texto']}" for a in i["alineas"] if not a["revogado"] and a["texto"].strip(" ;."))
        itexto = (i["texto"] + " " + ali_txt).strip() if ali_txt else i["texto"]
        add(make_entry(law, icode, artigo_num, pnum, i["num"], None, idesc, itexto, tipo, ipmin, ipmax, ifmt, "inciso", art))
        for a in i["alineas"]:
            if a["revogado"] or not a["texto"].strip(" ;."):
                continue
            acode = f"{icode}.{a['letra']}"
            aidu = f"{code}:{acode}"
            afmt = f"{ifmt}, {a['letra']})"
            apmin, apmax, _ = effective_pena(a, (ipmin, ipmax))
            adesc = DESC.get(aidu) or f"{parent_desc} – {short(a['texto'], 80)}"
            add(make_entry(law, acode, artigo_num, pnum, i["num"], a["letra"], adesc, a["texto"], tipo, apmin, apmax, afmt, "alinea", art))


def main():
    for law in LAWS:
        before = len(entries)
        build_law(law)
        print(f"{law[1]:>4} {law[2]:<22} +{len(entries) - before}")
    # ---- validações
    ids = [e["idUnico"] for e in entries]
    dup = [k for k, v in Counter(ids).items() if v > 1]
    if dup:
        print("DUPLICADOS:", dup[:20])
        sys.exit(1)
    if os.path.exists(OUT):
        old = json.load(open(OUT, encoding="utf-8"))
        old_ids = {a["idUnico"] for a in old}
        missing = sorted(old_ids - set(ids))
        print("idUnicos do seed anterior ausentes no novo:", missing)
    print("TOTAL:", len(entries))
    print("por tipo:", Counter(e["tipo"] for e in entries))
    print("por nivel:", Counter(e["nivel"] for e in entries))
    print("por legislacao:", Counter(e["legislacao"] for e in entries))
    empty_desc = [e["idUnico"] for e in entries if not e["descricao"]]
    print("sem descricao:", empty_desc[:20])
    with open(OUT, "w", encoding="utf-8") as f:
        json.dump(entries, f, ensure_ascii=False, indent=2)
        f.write("\n")
    print("escrito", OUT)


if __name__ == "__main__":
    main()
