# -*- coding: utf-8 -*-
"""Baixa os textos compilados oficiais do Planalto usados pelo gerador do seed penal."""
import os
import sys
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
SRC = os.path.join(HERE, "src")

URLS = {
    "cp_planalto": "https://www.planalto.gov.br/ccivil_03/decreto-lei/del2848compilado.htm",
    "lcp": "https://www.planalto.gov.br/ccivil_03/decreto-lei/del3688.htm",
    "drg": "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2006/lei/l11343.htm",
    "amb": "https://www.planalto.gov.br/ccivil_03/leis/l9605.htm",
    "des": "https://www.planalto.gov.br/ccivil_03/leis/2003/l10.826.htm",
    "mdp": "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2006/lei/l11340.htm",
    "eca": "https://www.planalto.gov.br/ccivil_03/leis/l8069.htm",
    "ctb": "https://www.planalto.gov.br/ccivil_03/leis/l9503compilado.htm",
    "cdc": "https://www.planalto.gov.br/ccivil_03/leis/l8078compilado.htm",
    "lvd": "https://www.planalto.gov.br/ccivil_03/leis/l9613.htm",
    "hed": "https://www.planalto.gov.br/ccivil_03/leis/l8072.htm",
    "l8137": "https://www.planalto.gov.br/ccivil_03/leis/l8137.htm",
    "rac": "https://www.planalto.gov.br/ccivil_03/leis/l7716.htm",
    "tort": "https://www.planalto.gov.br/ccivil_03/leis/l9455.htm",
    "orcrim": "https://www.planalto.gov.br/ccivil_03/_ato2011-2014/2013/lei/l12850.htm",
    "abuso": "https://www.planalto.gov.br/ccivil_03/_ato2019-2022/2019/lei/l13869.htm",
    "terror": "https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2016/lei/l13260.htm",
    "geno": "https://www.planalto.gov.br/ccivil_03/leis/l2889.htm",
    "idoso": "https://www.planalto.gov.br/ccivil_03/leis/2003/l10.741.htm",
    "sfn": "https://www.planalto.gov.br/ccivil_03/leis/l7492.htm",
    "falim": "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2005/lei/l11101.htm",
    "pcd": "https://www.planalto.gov.br/ccivil_03/_ato2015-2018/2015/lei/l13146.htm",
    "transp": "https://www.planalto.gov.br/ccivil_03/leis/l9434.htm",
    "ecopop": "https://www.planalto.gov.br/ccivil_03/leis/l1521.htm",
    "solo": "https://www.planalto.gov.br/ccivil_03/leis/l6766.htm",
    "soft": "https://www.planalto.gov.br/ccivil_03/leis/l9609.htm",
    "bio": "https://www.planalto.gov.br/ccivil_03/_ato2004-2006/2005/lei/l11105.htm",
    "intercep": "https://www.planalto.gov.br/ccivil_03/leis/l9296.htm",
    "hiv": "https://www.planalto.gov.br/ccivil_03/_ato2011-2014/2014/lei/l12984.htm",
    "prefeitos": "https://www.planalto.gov.br/ccivil_03/decreto-lei/del0201.htm",
    "l9029": "https://www.planalto.gov.br/ccivil_03/leis/l9029.htm",
    "eleitoral": "https://www.planalto.gov.br/ccivil_03/leis/l4737compilado.htm",
    "propind": "https://www.planalto.gov.br/ccivil_03/leis/l9279.htm",
    "cvm": "https://www.planalto.gov.br/ccivil_03/leis/l6385.htm",
    "henry": "https://www.planalto.gov.br/ccivil_03/_ato2019-2022/2022/lei/l14344.htm",
    "l7853": "https://www.planalto.gov.br/ccivil_03/leis/l7853.htm",
}


def main():
    os.makedirs(SRC, exist_ok=True)
    only = set(sys.argv[1:])
    for name, url in URLS.items():
        if only and name not in only:
            continue
        req = urllib.request.Request(url, headers={"User-Agent": "Mozilla/5.0 (retech-core penal-seed)"})
        with urllib.request.urlopen(req, timeout=60) as r:
            data = r.read()
        with open(os.path.join(SRC, name + ".html"), "wb") as f:
            f.write(data)
        print(f"{name:<12} {len(data):>8} bytes")


if __name__ == "__main__":
    main()
