# Esterium ERP — licenças e redistribuição dos componentes Windows

Este documento aplica-se ao instalador criado pelo projeto MIT Esterium ERP.
A licença MIT do repositório NÃO altera a licença dos programas de terceiros.

## Versões atualmente fixadas para o protótipo

| Componente | Origem | Versão | Observações |
|---|---|---|---|
| Esterium ERP | Este repositório | Em desenvolvimento | Código-fonte sob MIT (arquivo LICENSE na raiz) |
| PostgreSQL Server | PostgreSQL Global Development Group | 16.15 | PostgreSQL License; preservar os avisos aplicáveis |
| Binários PostgreSQL para integração Windows | EnterpriseDB (EDB) / PostgreSQL Global Development Group | postgresql-16.15-5-windows-x64-binaries.zip | Rota de instalação ZIP validada por SHA-256 fixo; preservar a PostgreSQL License e revisar/acompanhar avisos dos componentes incluídos no arquivo. |
| Instalador PostgreSQL para Windows | EnterpriseDB (EDB) | postgresql-16.15-1-windows-x64.exe | O executável de instalação é distribuído pela EDB: sua redistribuição em um pacote de terceiros requer revisão específica |
| Garnet | microsoft/garnet | 2.2.3 | MIT; distribuir com seu LICENSE e com os avisos de terceiros de NOTICE.md |
| .NET Runtime | Microsoft | 10.0.14 Windows x64 | MIT; archive SHA-512 pinned; preserve `LICENSE.txt` and `ThirdPartyNotices.txt` with the runtime |
| NSSM | nssm.cc | 2.24-101-g897c7ad | Domínio público, segundo o autor; preservar a atribuição e a origem por transparência |

## Verificações e política de publicação

* Binários fixados são conferidos por SHA-256 conhecido ANTES de integrar o
  pacote; a rota EDB EXE também exige certificado Authenticode válido do
  fornecedor aprovado. O ZIP oficial não tem assinatura Authenticode: sua
  verificação usa o hash independente publicado para esse arquivo exato.
* O hash SHA-256 do EXE de saída deve ser divulgado junto à release. Como o EXE
  é unsigned, isso comprova somente igualdade dos bytes com o hash obtido
  externamente, não a identidade do editor.
* NÃO anexar em GitHub Releases um executável contendo o EXE do instalador EDB
  até serem verificadas as permissões e condições específicas de redistribuição.
  A licença de código aberto do PostgreSQL Server não autoriza automaticamente
  a redistribuição de cada instalador ou componente de terceiros.
* A rota ZIP do pacote assinado inclui exclusivamente `16.15-5` e valida o
  SHA-256 publicado `43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281`.
  Os componentes PostgreSQL permanecem sujeitos à PostgreSQL License e seus
  avisos; revisar o inventário do ZIP e preservar avisos de dependências antes
  da distribuição pública. A rota EDB EXE continua distinta e requer revisão
  específica de redistribuição.
* Nunca publicar material de credencial (.env, .key, .pem, .pfx),
  bancos de dados, backups ou dados de clientes.
* O pacote não será marcado como instalador funcional até passarem os testes
  das rotas ZIP e EDB em Windows descartável, incluindo banco, serviços e
  recuperação. O teste do runner não substitui aceite de loja ou fiscal.

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
* .NET Runtime 10.0.14 Windows x64 release metadata:
  https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json
* NSSM licença/domínio público:
  https://www.nssm.cc/download

**Estado:** inventário preliminar para engenharia, não parecer jurídico.
