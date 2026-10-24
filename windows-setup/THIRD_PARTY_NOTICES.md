# Esterium ERP — licenças e redistribuição dos componentes Windows

Este documento aplica-se ao instalador criado pelo projeto MIT Esterium ERP.
A licença MIT do repositório NÃO altera a licença dos programas de terceiros.

## Versões atualmente fixadas para o protótipo

| Componente | Origem | Versão | Observações |
|---|---|---|---|
| Esterium ERP | Este repositório | Em desenvolvimento | Código-fonte sob MIT (arquivo LICENSE na raiz) |
| PostgreSQL Server | PostgreSQL Global Development Group | 16.15 | PostgreSQL License; preservar os avisos aplicáveis |
| Instalador PostgreSQL para Windows | EnterpriseDB (EDB) | postgresql-16.15-1-windows-x64.exe | O executável de instalação é distribuído pela EDB: sua redistribuição em um pacote de terceiros requer revisão específica |
| Garnet | microsoft/garnet | 2.2.3 | MIT; distribuir com seu LICENSE e com os avisos de terceiros de NOTICE.md |
| NSSM | nssm.cc | 2.24-101-g897c7ad | Domínio público, segundo o autor; preservar a atribuição e a origem por transparência |

## Verificações e política de publicação

* Binários fixados são conferidos por SHA-256 conhecido ANTES de integrar o
  pacote; o instalador EDB ainda exige certificado Authenticode válido do
  fornecedor aprovado.
* O hash SHA-256 do EXE de saída deve ser divulgado junto à release. Como o EXE
  é unsigned, isso comprova somente igualdade dos bytes com o hash obtido
  externamente, não a identidade do editor.
* NÃO anexar em GitHub Releases um executável contendo o EXE do instalador EDB
  até serem verificadas as permissões e condições específicas de redistribuição.
  A licença de código aberto do PostgreSQL Server não autoriza automaticamente
  a redistribuição de cada instalador ou componente de terceiros.
* O projeto PostgreSQL recomenda seus arquivos ZIP binários do Windows,
  sem instalador, para aplicações que incorporam PostgreSQL em seu próprio
  instalador. A migração para esse formato exigirá um novo processo de
  instalação e testes; não fingir compatibilidade com scripts que exigem EDB EXE.
* Nunca publicar material de credencial (.env, .key, .pem, .pfx),
  bancos de dados, backups ou dados de clientes.
* Mesmo após revisão de redistribuição, o pacote não será marcado como
  instalador funcional até serem testadas todas as operações com banco
  e serviços em Windows limpo e as recuperações após falhas.

## Fontes dos fornecedores

* PostgreSQL Windows (instaladores EDB e ZIP binário):
  https://www.postgresql.org/download/windows/
* PostgreSQL License:
  https://www.postgresql.org/about/licence/
* EDB:
  https://www.enterprisedb.com/docs/supported-open-source/postgresql/installing/windows/
* Garnet LICENSE:
  https://github.com/microsoft/garnet/blob/main/LICENSE
* Garnet NOTICE (dependências de terceiros):
  https://github.com/microsoft/garnet/blob/main/NOTICE.md
* NSSM licença/domínio público:
  https://www.nssm.cc/download

**Estado:** inventário preliminar para engenharia, não parecer jurídico.
