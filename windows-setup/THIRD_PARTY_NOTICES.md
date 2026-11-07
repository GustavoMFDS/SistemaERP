# Esterium ERP — licenças e redistribuição dos componentes Windows

Este inventário técnico acompanha o Setup do Esterium ERP. Ele não substitui
revisão jurídica nem muda a licença de nenhum fornecedor.

## Componentes e formato de distribuição

| Componente | Versão fixada | Licença/escopo |
|---|---|---|
| Esterium ERP | Em desenvolvimento | MIT; `LICENSE.txt` acompanha o Setup. |
| PostgreSQL para o instalador público | EDB `16.15-5` Windows x64 binaries ZIP | PostgreSQL License; hash SHA-256 fixo; o ZIP oficial inteiro fica embutido sem alteração. A extração é limitada a caminhos `pgsql/`. |
| Garnet | Microsoft Garnet `2.2.3` Windows x64 | MIT; acompanham `LICENSE` e `NOTICE.md` do tag fixado, incluindo os avisos das dependências listadas pelo projeto. |
| .NET Runtime | Microsoft `10.0.14` Windows x64 | MIT; `LICENSE.txt` e `ThirdPartyNotices.txt` vêm do runtime de hash SHA-512 fixo incluído no bundle. |
| WinSW | `2.12.0` NET4 x64 | MIT; `WinSW-2.12.0-LICENSE.txt` acompanha o Setup. O pacote também inclui avisos de `log4net` 2.0.12 (Apache-2.0) e `YamlDotNet` 8.1.2 (MIT), usados pela build do wrapper. |

O diretório `third-party-notices/` fica ao lado do executável e também dentro
do payload embutido. No build completo, ele inclui cópias dos avisos de cada
arquivo ZIP que tenha nome de licença, copyright ou notice; o inventário com
os caminhos encontrados é gerado pelo próprio build a partir dos arquivos
fixados.

O instalador EDB `postgresql-16.15-1-windows-x64.exe` é usado apenas na matriz
de compatibilidade de CI. Ele não deve ser incluído numa release pública até
uma revisão específica de seus termos de redistribuição. A rota escolhida para
uma futura release é o ZIP oficial, sem o instalador EDB.

## Integridade e publicação

* O ZIP PostgreSQL exato `16.15-5` é conferido antes do build pelo SHA-256
  `43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281`.
* Garnet, WinSW e o bundle ERP também são conferidos pelos hashes fixados antes
  de integrar o payload. Os arquivos de licença não alteram os binários.
* O runtime .NET precisa conter seus dois arquivos de avisos no bundle; o build
  falha se algum estiver ausente. Os textos também são copiados ao lado do EXE.
* O build falha se faltar qualquer texto estático de licença ou o inventário
  de avisos do arquivo PostgreSQL ZIP na rota `pgFormat=zip`.
* O EXE e os arquivos `LICENSE.txt`, `THIRD_PARTY_NOTICES.md` e
  `third-party-notices/` devem ser distribuídos juntos. Divulgue também o SHA-256
  do EXE; um hash confirma os bytes, mas não a identidade do editor.
* Não publicar credenciais, chaves privadas, bancos, backups ou dados de
  clientes.

## Fontes primárias

* PostgreSQL Windows e propósito do ZIP: https://www.postgresql.org/download/windows/
* PostgreSQL License: https://www.postgresql.org/about/licence/
* Garnet 2.2.3 LICENSE: https://raw.githubusercontent.com/microsoft/garnet/v2.2.3/LICENSE
* Garnet 2.2.3 NOTICE: https://raw.githubusercontent.com/microsoft/garnet/v2.2.3/NOTICE.md
* .NET 10 release metadata: https://builds.dotnet.microsoft.com/dotnet/release-metadata/10.0/releases.json
* WinSW v2.12.0 release and MIT license: https://github.com/winsw/winsw/releases/tag/v2.12.0
* WinSW v2.12.0 configuration and service-account docs: https://github.com/winsw/winsw/blob/v2.12.0/doc/xmlConfigFile.md
* log4net 2.0.12 license/notice: https://github.com/apache/logging-log4net/tree/rel/2.0.12
* YamlDotNet 8.1.2 license: https://github.com/aaubry/YamlDotNet/tree/v8.1.2

**Estado:** inventário de engenharia. A saída do próximo CI deve ser revisada
para confirmar os avisos encontrados no ZIP e Garnet fixados; isso não é um
parecer jurídico nem aprovação de redistribuição.
