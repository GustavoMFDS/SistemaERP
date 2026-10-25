# Esterium ERP - instalador Windows

Este diretorio contem o codigo do bootstrapper grafico EsteriumERP-Setup.exe e
o empacotador offline. Compila no Windows x64 com o compilador C# do .NET
Framework. No computador da loja nao exige Go, Docker nem Node.

## Ordem de preparo

1. Compilar backend e frontend via scripts/build-esterium-local-windows.ps1 em
   uma maquina de build Windows, gerando o bundle ERP.
2. Aprovar os pacotes oficiais PostgreSQL 16 EDB (EXE), Garnet (ZIP) e NSSM
   (EXE) por hash SHA-256; conferir tambem o certificado do EDB. A aprovacao
   deve vir de um canal independente do pacote.
3. Assinar todos os scripts e executaveis privilegiados do bundle ERP com o
   editor comercial, refazer o manifesto somente DEPOIS de assinar e aprovar
   seu SHA-256 por canal independente.
4. Gerar a interface tecnica SEM payload:

    powershell.exe -NoProfile -File .\windows-setup\build-esterium-setup-exe.ps1 -OutputDir C:\releases\setup-preview-novo

5. Para um EXE de PREVIA TECNICA com os binarios reais embutidos, sem
   comprar assinatura, usar -TechnicalPreviewPayload e os hashes de fontes
   confiaveis; o botao Instalar fica bloqueado mesmo se o arquivo for
   assinado posteriormente. Exige pacote ERP com manifesto valido, vendor
   PostgreSQL realmente assinado e hashes confiaveis dos tres fornecedores.
   O arquivo gerado recebe o nome EsteriumERP-Setup-TECHNICAL-PREVIEW.exe.
   Para a previa, reutilize os parametros do exemplo abaixo, mas SUBSTITUA
   -TrustedPublisherThumbprint EDITOR_APROVADO por -TechnicalPreviewPayload.
   A previa NUNCA declara um editor comercial: usa o manifesto tecnico real
   e os hashes independentes EDB, Garnet e NSSM. Nao permite instalacao.

6. Gerar o EXE UNICO comercial com o bundle assinado e os tres fornecedores:

    powershell.exe -NoProfile -File .\windows-setup\build-esterium-setup-exe.ps1 -OutputDir C:\releases\setup-completo-novo -BundleDir C:\releases\erp-assinado -ApprovedManifestSHA256 SHA256_APROVADO -TrustedPublisherThumbprint EDITOR_APROVADO -PostgresInstaller C:\vendors\postgresql.exe -ExpectedPostgresSHA256 SHA256_PG -ExpectedPostgresPublisherThumbprint EDITOR_PG -GarnetPackage C:\vendors\garnet.zip -ExpectedGarnetSHA256 SHA256_GARNET -ServiceWrapperExe C:\vendors\nssm.exe -ExpectedServiceWrapperSHA256 SHA256_NSSM

7. Assinar o EXE FINAL com o certificado Authenticode comercial do editor,
   validar a assinatura e SHA-256 e realizar piloto em Windows limpo.

## Comportamento e limites de seguranca

O EXE apresenta Verificar arquivos, Ver plano e Instalar. Se houver payload
embutido, extrai em pasta temporaria exclusiva. Verifica hashes do manifesto
ERP e de todos os fornecedores antes de iniciar os scripts. O usuario deve
informar hashes e identidades obtidos fora do proprio EXE distribuido.

Instalar exige EXE final assinado com Authenticode valido. Executa o
coordenador tecnico existente install-esterium-local-complete.ps1 com
PowerShell AllSigned e UAC. O coordenador valida novamente os scripts, as
assinaturas, dados de origem HTTPS, servicos e banco. Nao ativa vendas, nao
habilita SEFAZ e nao repete automaticamente uma instalacao parcialmente
falhada. Nunca use hashes inventados ou opcao de fixture em loja.

O EXE sem payload serve apenas como preview de build. Um bundle ainda nao
assinado NAO passa pelo empacotador de distribuicao.

Ainda pendente antes de vender: assinatura do editor, fornecedores aprovados
reais, teste em Windows novo, HTTPS dos terminais, backup restaurado e
verificacao fisica de reboot e recuperacao.


## Distribuicao MIT sem certificado pago (plano seguro)

A MIT do repositorio rege o codigo Esterium; as licencas dos fornecedores
permanecem independentes. O bootstrapper pode ser compilado sem assinatura
paga. O modo `-TechnicalPreviewPayload` EMBUTE os arquivos reais, hashes
SHA-256 da versao e thumbprint do EDB como metadados de conferencia;
nao embute nem presume certificado confiavel para o editor ERP.

No computador Windows, a interface permite **Verificar arquivos** e
**Ver plano**, mas nunca **Instalar**. O plano e executado em PowerShell
nao elevado, com `-ExecutionPolicy RemoteSigned` para avaliar scripts
locais cujos bytes foram conferidos contra o manifesto SHA-256.
Nao altera a politica permanente do Windows nem remove GPOs empresariais. O
coordenador nao recebe `-Apply`. A protecao por hash nao garante
autenticidade do publicador: compare o SHA-256 do EXE com o release
original no GitHub antes de executar. Nunca instale uma CA raiz ou
altere politica global de PowerShell para testar o pacote.

O plano pode falhar legitimamente em maquina ja usada: as verificacoes
interrompem se pastas, portas, contas e servicos indicarem instalacao
anterior. Nao apague os dados anteriores nem tente contornar essa protecao.
O campo de HTTPS deve receber a origem real da loja, previamente definida.

O caminho real para INSTALAR sem certificado comercial ainda precisa de
uma cadeia de verificacao de integridade antes de executar cada delegado
privilegiado, testes de ponta a ponta em Windows descartavel e prova de
recuperacao/backup. Os scripts atuais de `-Apply` da release comercial
exigem assinatura valida dos proprios scripts e continuam intactos.
Esta etapa nao pretende remover ou contornar essa exigencia.

Garnet (MIT), NSSM (dominio publico), PostgreSQL e o empacotamento EDB
tem seus proprios termos. Revisar os avisos e a autorizacao de
redistribuicao de binarios de terceiros, em especial o EXE do EDB,
antes de fazer upload de pacote offline grande nos Releases. O
PostgreSQL.org recomenda ZIP de binarios proprio para incorporacao em
outros instaladores.

A previa publica pequena em GitHub Releases nao tem componentes
embutidos e continua sem instalacao nem plano funcional.


## Preparar arquivos em Program Files — MIT sem assinatura (Fase 1)

A versão -TechnicalPreviewPayload com todos os arquivos permite agora
ações separadas:

- Verificar arquivos: confere manifesto ERP e os hashes de PostgreSQL,
  Garnet e NSSM, sem privilégios de administrador.
- Ver plano: confere o fluxo existente de dependências e bootstrap em
  modo somente leitura, sem -Apply.
- Preparar arquivos (não instala): após confirmação e UAC, o próprio
  EXE recompõe o conteúdo ERP a partir do ZIP integrado, valida
  o manifesto e todos os hashes de fornecedores registrados no build
  e copia SOMENTE o ERP para C:\Program Files\EsteriumOpenSourceStage.
  Impede sobrescrever a pasta, configura ACL restrita para SYSTEM e
  Administradores, valida os hashes depois da cópia e depois da renomeação.

A fase de preparação nunca executa o instalador EDB, nem cria contas locais,
serviços, segredos, banco ou configurações de rede. O botão Instalar
continua bloqueado. Preparar arquivos NÃO equivale a instalar o ERP.
Uma segunda execução é recusada para proteger os dados já preparados.

O processo elevado usa um comando nativo do mesmo executável
(--stage-oss-files), reextrai e revalida os recursos incorporados e jamais
executa um script PowerShell temporário com privilégios de administrador.
O arquivo técnico não é assinado: essa medida protege a integridade dos
arquivos comparados aos hashes embutidos, mas NÃO prova a identidade
do publicador. Verifique também o hash do EXE contra o valor fornecido
por fonte independente confiável.

Teste disponível em test-unsigned-real-plan-ci.ps1 no GitHub Actions
Windows hospedado: build real e download/assinatura/hash dos fornecedores,
plano não destrutivo, construção do EXE integrado, staging real em
Program Files, conferência de hashes, rejeição de repetição e ausência
de serviços. O teste NÃO deve ser executado no computador de uma loja.

A próxima fase de instalação de PostgreSQL/Garnet e SQL continua bloqueada,
pois os coordenadores comerciais existentes exigem assinaturas Authenticode.
Não é permitido contornar ou simular essas assinaturas. O empacotador
de fornecedores e as condições de redistribuição da EDB precisam ser
revisados antes do upload do EXE integrado a Releases públicos.


## Preparar dependências em Program Files — MIT unsigned (Fase 2A)

Depois de executar com sucesso "Preparar arquivos (não instala)", o
instalador integrado oferece "Preparar dependências (não instala)".
Essa operação pede confirmação e UAC, verifica novamente o manifesto
do ERP já preparado em Program Files e os hashes dos binários
originais embutidos, inclusive a assinatura Authenticode REAL do
instalador EDB PostgreSQL com identidade de publicador previamente
fixada. Depois guarda exatamente:

- postgresql.exe — EDB original, assinatura válida e SHA-256 fixado;
- garnet.zip — arquivo de fornecedor cujo SHA-256 foi fixado;
- nssm.exe — binário de fornecedor cujo SHA-256 foi fixado.

Pasta final isolada: C:\Program Files\EsteriumOpenSourceVendorStage,
com ACL sem herança que concede acesso somente ao SYSTEM e aos
Administradores. Os bytes são comparados antes e depois da cópia.
Uma segunda execução recusa sobrescrever arquivos existentes.
Não é permitido preparar fornecedores se o ERP preparado não estiver
intacto ou se qualquer assinatura ou hash esperado divergir.

**NÃO executa os fornecedores, NÃO configura PostgreSQL/Garnet, NÃO cria
contas, credenciais, certificados, dados SQL ou serviços.** Isso exige
uma etapa adicional de instalação propriamente dita, que continuará
protegida e terá de ser validada de ponta a ponta em ambiente descartável.

Teste CI sob workflow_dispatch verifica cenário real com os binários
fixados, valida as ACL, os hashes e a assinatura EDB pós-cópia,
exige recusa em segunda execução e confirma que serviços não surgiram.
Este teste NUNCA deve ser executado no PC de uma loja.


## Teste de instalação REAL do PostgreSQL EDB no GitHub Actions

O workflow manual de Windows usa o teste
test-real-edb-install-disposable-ci.ps1 depois de verificar o ERP e os
fornecedores. Diferentemente do plano sem -Apply, este teste **executa
o instalador EDB original**, mas APENAS numa máquina temporária nova
do GitHub Actions: nunca no equipamento do usuário ou em loja.

O script exige Windows, github-hosted runner, a variável explícita
ESTERIUM_REAL_EDB_DISPOSABLE_CI=1 e privilégio de Administrador.
Ele valida de novo o hash SHA-256 e a assinatura Authenticode do EDB
antes da execução, cria senhas aleatórias distintas somente para
esse runner (nunca as imprime), grava as opções EDB num arquivo com
ACL restrita a SYSTEM/Administradores e usa destinos, conta e serviço
**exclusivos de CI**, sem reutilizar nenhuma instalação anterior.
A opção EDB é apagada em bloco finally.

Mesmo se o instalador retornar erro, o teste tenta desabilitar e
parar o serviço criado. O sucesso só é declarado quando existe
PG_VERSION=16, os executáveis esperados, a identidade da conta
dedicada, o serviço parado/desabilitado e nenhuma porta aberta.

**Limitações:** não instala ou libera os serviços da loja. Não
substitui a adaptação e homologação de toda a cadeia de migrações,
TLS, serviço ERP, backups e recuperação. É proibido executar esse
teste fora do CI descartável. A instalação comercial com -Apply
continua exigindo Authenticode dos scripts.


## Pré-checagem de Windows limpo (somente leitura)

A prévia integrada MIT inclui o botão "Pré-checar Windows (não instala)"
e o comando técnico --preflight-oss. O recurso exige que o ERP e os
fornecedores já estejam preparados e que o processo possa ler as
pastas protegidas com autorização de Administrador.

Antes de qualquer futura instalação real, a pré-checagem:
- relê o manifesto e cada SHA-256 do ERP;
- exige exatamente os três fornecedores esperados, seus hashes pinados
  e a assinatura Authenticode válida do EXE EDB;
- recusa pastas de dados/opções/instalação já existentes;
- recusa serviços Windows Esterium existentes, conta dedicada
  esterium_pgsvc existente, e portas 5432, 6379 e 8080 ocupadas.

Não executa pacotes de fornecedor, não cria arquivos, não altera
configurações, serviços, firewall, bancos ou usuários. Um Windows onde
uma instalação parcial já ocorreu precisa de avaliação manual, nunca
limpeza automática. O teste CI exige preflight aprovado ANTES de criar
o novo serviço EDB descartável, e preflight recusado DEPOIS.


## Senhas fortes e compatibilidade com política Windows

O teste de instalação real da EDB detectou que senhas aleatórias
somente em hexadecimal (maiúsculas e dígitos) não são aceitas por
políticas Windows que exigem caracteres de categorias diferentes.
A geração no teste EDB e a geração de segredos do bootstrap Esterium
foram adaptadas para sempre conter maiúscula, minúscula, número e
pontuação permitida, mantendo 48 bytes independentes de entropia CSPRNG
e o armazenamento em arquivos protegidos. Não há senha fixa:
somente o prefixo que garante as categorias é constante. Nenhuma senha
é escrita no console, nos logs ou no GitHub Release.

Esta alteração não equivale à aprovação de uma instalação completa.


## PostgreSQL no instalador MIT: alternativa oficial em ZIP (investigação)

O teste CI de instalação *real* do `postgresql-16.15-1-windows-x64.exe`
encontrou um problema de política de senha, corrigido no gerador, mas a
execução seguinte **travou e atingiu o limite de 8 minutos** no Windows
descartável (run 40167419917). Portanto, esse instalador EDB ainda NÃO
foi homologado para uso automático na release MIT.

O projeto PostgreSQL documenta que binários Windows em ZIP são
destinados a integrações com instaladores de outras aplicações:
https://www.postgresql.org/download/windows/

A EDB oferece `postgresql-16.15-5-windows-x64-binaries.zip`.
O SHA-256 fixado da edição **16.15-5**, divulgado por um colaborador
do projeto EDB e conferido diretamente após o download, é:
`43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281`.
Referência: https://github.com/EnterpriseDB/edb-installers/issues/731 .
**Atenção:** o SHA `80379...` na mesma discussão é da edição
**PostgreSQL 17.11-5**, e NÃO deve ser usado com 16.15.

Validação técnica observada no Windows do desenvolvedor: ZIP de
373.254.386 bytes com 21.799 entradas, incluindo `pgsql/bin/postgres.exe`,
`initdb.exe`, `pg_ctl.exe`, `psql.exe` e `pg_isready.exe`, sem
symlinks, e correspondência exata ao SHA pinado. **Nenhum executável do
ZIP foi usado para instalar serviços nem inicializar o banco no PC.**

O novo teste de CI `test-postgres-oss-binaries-ci.ps1` verifica o
hash independentemente fixado, caminhos do arquivo compactado e
`--version` dos executáveis extraídos em Windows descartável.
Ele não executa `initdb`, não configura credenciais nem serviços.

A antiga experimentação de instalação do EDB EXE permanece acessível
somente por ação manual explícita
`run_experimental_edb_install=true` no workflow. Está separada do
fluxo preferencial do ZIP para NÃO mascarar o histórico de falhas.

**Próximo desenvolvimento:** implementar instalação real do PostgreSQL
diretamente a partir do ZIP com conta não privilegiada, initdb, registro
de serviço desabilitado, TLS, SQL, Garnet e backups, seguido de teste
E2E completo. Até lá, o botão **Instalar** da versão MIT continua
bloqueado e nenhum ZIP/EXE integrado foi publicado como release.


## Um unico EXE tecnico MIT com ZIP PostgreSQL (nao instalavel)

O builder aceita exclusivamente no modo `-TechnicalPreviewPayload` o switch
`-TechnicalPreviewPostgresBinaryZip`, usando o caminho de
`-PostgresInstaller` para o arquivo
`postgresql-16.15-5-windows-x64-binaries.zip` e o hash
`-ExpectedPostgresSHA256 43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281`.
Neste modo o thumbprint
`-ExpectedPostgresPublisherThumbprint` deve estar vazio; o ZIP nao
e um executavel Authenticode e sua autenticidade depende da comparacao
do SHA-256 obtido de fonte de fornecedor independente.
O build comercial existente continua EXE EDB + Authenticode.

O unico executavel tecnicamente integrado passa a carregar o recurso
`vendor/postgresql-binaries.zip` ao lado de Garnet/NSSM e do ERP.
Ele verifica a integridade do arquivo interno, a lista e os caminhos
das entradas PostgreSQL ZIP, os binarios essenciais, o manifesto ERP
e os fornecedores. A preparacao de arquivos e fornecedores continua
restrita a Program Files, com hashes e ACLs protegidas. A rotina
de plano PowerShell antigo (exige EDB EXE) fica DESATIVADA neste modo
para evitar apresentar um plano incompativel.

No Windows descartavel o teste integrado
https://github.com/GustavoMFDS/SistemaERP/actions/runs/40171137153
confirmou preparacao de ERP e fornecedores com ZIP, hashes, pastas
protegidas, idempotencia negativa e preflight limpo.
O job de interface deste run falhou apenas pela ausencia do campo
opcional pgFormat na fixture antiga; a leitura foi corrigida e
reequilibrada pelos testes subsequentes. A validacao negativa de hash
incorreto agora e testada pelo CI do pacote integrado.

Este artefato **NAO instala PostgreSQL nem inicia banco**. O teste
separado do ZIP com initdb, aprovado no runner, nao esta ligado ao
botao Instalar: as fases persistentes de conta Windows, cluster,
servicos, SSL/TLS, schema, Garnet e backup continuam pendentes.
A distribuicao publica precisa reter licencas/avisos de terceiros e
ser revisada antes de promover o arquivo a Release.
