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


## PostgreSQL ZIP: registro do serviço Windows no CI descartável

O teste de `initdb` no Windows hospedado executa uma etapa adicional
em `test-postgres-oss-service-ci.ps1`: registra no Windows Service
Control Manager um serviço PostgreSQL com nome aleatório exclusivo,
usando os binários `pg_ctl.exe` do ZIP cujo SHA-256 já foi validado.
A conta é a mesma identidade local sem privilégios de Administrador
que criou o cluster. O registro usa `New-Service -Credential` com
`PSCredential`, de modo que a senha não é transmitida na linha de
comando do processo.

O serviço começa com tipo de inicialização **Disabled** e estado
**Stopped**. O teste confere no SCM o caminho do executável, o nome da
conta, a ausência de processo em execução e a ausência de alterações
nos nomes de serviços do Esterium de produção; remove o serviço único
de CI antes de apagar a conta descartável.

**Isso não libera instalar o ERP em lojas.** Ainda não é teste de
funcionamento PostgreSQL + Garnet + ERP, TLS, backup, migrações ou
recuperação após falha; tampouco executa serviços no PC de usuário.
O script recusa execução fora do GitHub Actions hospedado com a
variável `ESTERIUM_PG_ZIP_DISPOSABLE_CI=1`.


## Startup real do servico PostgreSQL no Windows descartavel

O primeiro teste com `New-Service -Credential` registrou o servico
PostgreSQL como conta Windows sem direitos de administrador, desabilitado
e parado, e confirmou sua remocao. Ao tentar iniciar o servico, o
Windows devolveu **codigo 15 (falha de logon no servico)**, o que
explicou a diferenca entre apenas registrar e realmente executar.

No teste `test-postgres-oss-lsa-ci.ps1`, a conta **unica e efemera**
de CI recebe temporariamente `SeServiceLogonRight` via API LSA do
Windows. O script recusa execucao fora do runner GitHub hospedado,
confirma SID de usuario local e prefixo de conta `epg`, e nao
usa `secedit` para substituir politicas existentes.

O teste de ciclo de vida agora tenta: registrar servico Disabled,
alterar temporariamente para Manual **somente no runner**, iniciar
em `127.0.0.1` e porta aleatoria, consultar `SELECT 1` usando
uma credencial efemera nunca registrada em argumento CLI ou logs,
parar, desabilitar, excluir servico, revogar a permissao LSA e remover
a conta criada.

**Nao confundir o teste Windows CI com instalacao aprovada no cliente.**
A versao MIT publica continua com instalacao e migracoes bloqueadas,
sem TLS e sem testes de backup/restauracao. A operacao de permissão
LSA nao faz parte do EXE distribuido aos clientes.


## PostgreSQL ZIP — teste de serviço, SQL e backup/restore validado

Em GitHub Actions Windows **descartável** (run
https://github.com/GustavoMFDS/SistemaERP/actions/runs/40237011076)
o script `test-postgres-oss-service-ci.ps1` concluiu:

1. `initdb` em uma conta Windows **sem privilégios de Administrador**;
2. Concessão temporária de `SeServiceLogonRight` **somente** para a conta
   efêmera de CI via LSA; registro do serviço PostgreSQL com
   `New-Service -Credential` sem senha em argumentos;
3. Serviço manual apenas durante o teste, escutando em `127.0.0.1`
   e porta aleatória. Consulta SQL autenticada `SELECT 1`;
4. Criação de banco novo exclusivamente de teste, inserção de uma linha,
   backup real via `pg_dump -Fc`, exclusão do banco, recriação e
   restauração real via `pg_restore`, seguida da conferência do registro;
5. Parada e desativação do serviço, verificação da porta fechada,
   exclusão do serviço único de CI, revogação do direito de logon
   temporário e exclusão da conta após o teste.

A CI nunca usa contas, nomes de serviços, banco de dados, certificados
ou arquivos de lojas. O teste exige `GITHUB_ACTIONS=true`,
`RUNNER_ENVIRONMENT=github-hosted` e
`ESTERIUM_PG_ZIP_DISPOSABLE_CI=1` e rejeita executar fora disso.
Nenhuma senha foi incluída no console nem nos argumentos dos processos.

**Ainda pendente para clientes:** integrar esse processo em uma
instalação persistente confiável no EXE MIT (fluxo privilegiado
independentemente validado), configurar TLS, Garnet, serviços do ERP,
migrações, prova de integridade fiscal, backup externo/restore de
instalação completa, recuperação de falhas e homologação em máquina
Windows limpa. O botão Instalar continua desabilitado nas prévias.


## Validação integrada dos componentes MIT no Windows (25/10/2026)

O workflow manual `.github/workflows/esterium-windows-setup.yml` agora reutiliza
testes *reais* no Windows GitHub hospedado, mantendo os componentes isolados:

- `postgresql-oss-binaries-smoke`: ZIP original PostgreSQL 16.15-5 com
  SHA-256 publicado pela EDB, conta Windows sem privilégios, cluster inicializado,
  serviço iniciado somente em loopback, consulta SQL, `pg_dump`, `pg_restore`
  e limpeza de serviço/conta/permissões;
- `garnet-oss-real-tls-aof-smoke`: Garnet 2.2.3 com TLS, ACL negando
  autenticação indevida, Lua, AOF recuperado após encerramento forçado,
  NSSM executado como NETWORK SERVICE e serviço finalmente parado/desativado;
- `postgres-oss-tls-schema-smoke`: ZIP 16.15-5 com TLS verify-full, SCRAM,
  conta de aplicação do ERP sem privilégios, migrações reais, rejeição de
  replay e recuperação do banco em outro cluster. Teste de
  https://github.com/GustavoMFDS/SistemaERP/actions/runs/40242731044 aprovado;
- `unsigned-integrated-zip-preview`: compila e extrai/verifica ERP e
  fornecedores em um único EXE sem assinatura; verifica hashes,
  preparação de arquivos com ACLs e negativa contra adulterações.
- `unsigned-preview`: build e testes negativos de instalação.

O job `oss-component-gate` é uma condição fail-closed: exige
`success` em **todos** os cinco testes anteriores. Testes
ignorados/cancelados/falhos NÃO aprovam o conjunto.

**NÃO significa que o instalador de clientes está pronto.** O
workflow não realiza, numa única instalação persistente, o bootstrap
completo de PostgreSQL+Garnet+ERP, recuperação após reboot,
backup externo/restauração do sistema da loja ou teste fiscal.
O fluxo MIT continua com `Instalar` desabilitado e a PR em draft.


## Fase 3 MIT: extracao protegida de binarios PostgreSQL (nao instala)

A interface tecnica ZIP inclui **"Preparar binarios PostgreSQL (nao instala)"**
e o comando `--stage-oss-postgres`, disponiveis **somente** na previa
MIT com payload completo e ZIP oficial 16.15-5.

A operacao exige UAC e roda *preflight* novamente: validacao do
manifesto ERP preparado, ZIP oficial PostgreSQL e demais fornecedores,
pastas/servicos/contas/portas sem conflitos. Abre o mesmo ZIP em um
handle sem compartilhamento de escrita, confere seu SHA-256 fixado
`43BB45F173A6F08CF1D29A97A6D8DEB119E8E8093A24C00D2D1001A0CCAA8281`
e somente depois extrai para um diretorio unico protegido em
`C:\\Program Files`. Recusa entradas ZIP com caminhos inseguros,
links simbolicos, duplicatas e arquivos alem dos limites de tamanho.
Compara os hashes de todos os arquivos extraidos depois da renomeacao
para `C:\\Program Files\\EsteriumOpenSourcePostgresBin\\pgsql`,
com ACL que concede acesso apenas a SYSTEM/Administradores.
Se o diretorio final existir, a operacao e recusada: nao substitui
nem apaga binarios e dados anteriores.

O CI sintético testa que a previa sem payload ou com bytes de
fornecedor inventados NAO possa preparar os binarios. O CI manual
`test-unsigned-real-plan-ci.ps1 -PostgresBinaryZip` foi ampliado
para executar essa preparacao com o ZIP verdadeiro, conferir os
executaveis extraidos, versao, ACL, recusa de repeticao e ausencia
de servicos.

**Limite importante:** essa etapa so extrai binarios confiaveis.
NAO chama `initdb`, NAO registra servico, NAO cria conta local,
NAO abre rede, NAO aplica migracoes e NAO habilita instalacao comercial.
O binario do computador local nao foi recompilado automaticamente;
a integracao precisa passar no Windows GitHub CI antes da entrega.


## 25/10/2026 — E2E de banco a partir dos binários que o EXE realmente preparou

O job `unsigned-integrated-zip-preview` na PR passou a executar,
**somente em Windows GitHub hospedado descartável**, a cadeia:

1. Empacota e verifica o próprio EXE integrado com ERP + ZIP original
   PostgreSQL 16.15-5 + Garnet/NSSM;
2. O próprio EXE `--stage-oss-files`, `--stage-oss-vendors`,
   `--preflight-oss`, `--stage-oss-postgres` extrai/verifica os
   binários dentro de Program Files, sem criar serviços/contas;
3. Verifica ACLs e `--version` das cópias resultantes. Exige bloqueio
   de repetição e ausência de serviços do produto;
4. Com `GITHUB_ACTIONS=true`, `RUNNER_ENVIRONMENT=github-hosted`,
   `ESTERIUM_OPEN_SOURCE_PLAN_DISPOSABLE_CI=1` e
   `ESTERIUM_PG_ZIP_DISPOSABLE_CI=1`, executa exclusivamente
   `test-postgres-oss-initdb-ci.ps1 -FromIntegratedStage` sobre
   `C:\Program Files\EsteriumOpenSourcePostgresBin\pgsql`, e NÃO
   sobre uma segunda cópia ZIP separada. O teste cria conta sem
   privilégios para um cluster efêmero sob RUNNER_TEMP;
5. Essa conta recebe temporariamente somente leitura/execução dos
   binários. A execução usa o serviço Windows isolado, realiza
   `SELECT 1`, `pg_dump -Fc`, `pg_restore`, confere o dado
   restaurado e para/exclui o serviço. A permissão de logon e a
   leitura adicional concedida ao usuário temporário são revogadas.

O script com esse teste adicional é rejeitado se for chamado fora
do runner CI hospedado. **Ainda não é um método de instalação
persistente de clientes**: não cria serviço `EsteriumPostgreSQL16`,
não executa Garnet nem ERP no mesmo host e não habilita `Instalar`.
A validação dessa cadeia está em:
https://github.com/GustavoMFDS/SistemaERP/actions/runs/40254899332
(consulte o resultado antes de anunciar a cadeia aprovada).


## Testes fim a fim usando TODOS os bytes do mesmo EXE MIT (em andamento)

O workflow \`unsigned-integrated-zip-preview\` em Windows descartável
foi ampliado para testar de forma sequencial:

1. Preparação de ERP, fornecedores e ZIP PostgreSQL pelos comandos
   nativos do mesmo arquivo \`.exe\`, mantendo ACLs e hashes pinados;
2. PostgreSQL \`initdb\` sob uma conta local de CI sem privilégios, com
   data directory sob uma pasta **exclusiva do teste** em ProgramData,
   serviço temporário, SQL autenticado e pg_dump/pg_restore, revogando
   permissões temporárias após uso;
3. PostgreSQL TLS verify-full/SCRAM, migrações completas do ERP, WAL e
   restauração num segundo cluster, utilizando o executável \`initdb\`
   que o próprio \`.exe\` extraiu (sem segundo download ZIP);
4. Garnet TLS/ACL/Lua e AOF com recuperação após falha e serviço NSSM
   NetworkService, utilizando Garnet ZIP e NSSM que o EXE preparou.
   O arquivo NSSM é copiado para uma pasta CI somente-leitura do
   NetworkService, sem conceder acesso adicional ao vendor-stage;
5. Registro do \`esterium-server.exe\` real, vindo do bundle preparado
   pelo EXE, como serviço \`EsteriumERP\` **parado/manual**,
   LocalService e fiscal/SEFAZ desabilitados, com arquivos de
   credenciais sintéticos e ACLs restritas; o serviço não deve iniciar
   e é removido no fim do teste.

O passo 3 apresentou **uma falha em initdb** na primeira execução
integrada (run 40256012496), após o sucesso das etapas de PostgreSQL
não administrador/SQL/backup. Isso NÃO invalida os testes de componentes
independentes, mas **bloqueia qualquer declaração de sucesso do E2E
integrado**. O script foi atualizado para capturar apenas diagnóstico
CI redigido com substituição de senhas sintéticas por \`[REDACTED]\`,
a fim de identificar e corrigir a causa, em vez de pular a falha.
Executável cliente **ainda não instala** banco ou serviços.

O workflow \`oss-component-gate\` agora é fail-closed também em PR:
exige sucesso de \`unsigned-preview\` e do E2E integrado, enquanto
workflow manual requer os cinco jobs de componentes anteriores.
Um CI parcialmente aprovado ou qualquer job \`skipped\` que seja
obrigatório NÃO autoriza release.

**Atenção:** esses passos são exclusivas fixtures com variáveis
CI e Windows runner temporário. Não permitem executar scripts
administrativos unsigned no Windows de lojas. Integrar o fluxo
privilegiado real, testes de reboot, backup externo e auditoria
operacional continua pendente.


## Auditoria read-only dos binários PostgreSQL antes do bootstrap

O comando nativo \`--audit-oss-postgres\`, disponível apenas no EXE MIT
técnico com ZIP PostgreSQL embutido, NÃO altera o Windows. Ele exige
elevação somente para leitura de Program Files, revalida o hash externo
do ZIP original \`16.15-5\`, protege o handle de origem contra gravação
concorrente e confere **todos os arquivos extraídos**, um por um, contra
a versão correspondente de cada entrada aprovada no ZIP.
Também rejeita links simbólicos/junctions, arquivos extra,
binários essenciais ausentes e ACLs que concedam acesso fora de
SYSTEM/Administradores.

Essa auditoria será uma das pré-condições obrigatórias antes que
qualquer futuro fluxo de instalação passe a executar o PostgreSQL.
O CI \`test-unsigned-real-plan-ci.ps1 -PostgresBinaryZip\` testa
sucesso sobre a cópia intacta, recusa um arquivo adicional exclusivo
do teste, remove o marcador e exige sucesso novamente. O marcador
não é executável e não modifica bytes do PostgreSQL.

## Ciclo de integração ampliado e cache verificado

Além da criação do cluster não administrativo em ProgramData de CI,
o workflow procura executar TLS verify-full/SCRAM, migrações reais do
ERP e restauração em cluster independente usando \`initdb.exe\`
dentro do Program Files preparado pelo próprio EXE. Numa primeira
tentativa houve falha \`0xC0000135\` (Windows: componente DLL ausente)
na execução de \`initdb\` pelo PowerShell 7. A falha **não foi
silenciada**: o script agora fornece diagnóstico limitado, sem
credenciais, valida \`initdb.exe\` e \`postgres.exe\` no mesmo ambiente
e configura caminho explícito de DLLs \`pgsql/bin\` e \`pgsql/lib\`.
A rota integrada segue não homologada até obter CI verde.

O workflow também valida Garnet TLS/ACL/Lua/AOF e serviço NSSM
com os mesmos arquivos do EXE, e um teste de registro do ERP
\`esterium-server.exe\` real como LocalService parado/manual, com
dados de login fictícios, fiscal/SEFAZ desabilitado. Nenhum dos
testes altera o computador de usuários.

Por eficiência, o GitHub Actions pode reutilizar em \`.vendor-cache\`
o ZIP oficial PostgreSQL 16.15-5, mas trata cada cache como
**conteúdo NÃO CONFIÁVEL** e exige o SHA-256 independente original
antes e depois da cópia. Um cache adulterado bloqueia o teste.
Isso não autoriza distribuir o ZIP a clientes nem elimina os
avisos/licenças de terceiros.


## Etapa nativa 4 — preparar pastas protegidas em ProgramData (não instala)

A prévia MIT com ZIP oficial PostgreSQL dispõe da operação
`Preparar pastas de dados (não instala)` (CLI `--stage-oss-data`).
Ela **não usa a rota comercial de assinaturas Authenticode** e não
substitui sua verificação; trata apenas do layout inicial da rota
MIT experimental.

A etapa exige Administrador/UAC, repete a auditoria de integridade
de **todos** os binários PostgreSQL extraídos em Program Files, e
revalida manifesto ERP, Garnet e NSSM por hashes embutidos.
Recusa pasta `C:\ProgramData\Esterium` já existente e os serviços
`EsteriumPostgreSQL16`, `EsteriumGarnet` e `EsteriumERP`
quando registrados.

A pasta `C:\ProgramData\Esterium` é criada de forma atômica
com `CreateDirectoryW` e descritor de segurança explícito;
uma pasta preexistente (inclusive criada em corrida) **não é adotada**.
O DACL não herda permissões de ProgramData, autorizando apenas
SYSTEM e Administradores. São criados apenas diretórios **vazios**:
`PostgresData`, `GarnetData`, `Secrets` e `Backups`.
O arquivo `oss-data-layout.json` registra honestamente a fase
`DATA_DIRECTORIES_ONLY` com `databaseInitialized=false`,
`servicesInstalled=false` e `backupsConfigured=false`.
Não há segredos nesse registro.

Em caso de interrupção, **o estágio parcial é preservado para
diagnóstico**, não removido automaticamente. Uma nova tentativa
encontra a pasta já existente e é bloqueada. Isso evita apagar
qualquer dado de instalação anterior ao tentar recuperar de falha.

O teste de CI Windows descartável confirma criação, ausência
de arquivos nas quatro pastas, ACL sem herança, registro de estado,
recusa da segunda execução sem alterar o marcador e ausência de
serviços. A validação nova está em:
https://github.com/GustavoMFDS/SistemaERP/actions/runs/40422436873
(consultar a conclusão do job antes de anunciar como aprovada).

**Não executa** `initdb`, criação de contas de serviço, SQL,
Garnet, certificado TLS, registro de serviço, migrações ou backups.
Essas operações requerem implementação e testes adicionais para a
instalação persistente. O botão `Instalar` permanece bloqueado.

## Etapa nativa 5 — credenciais offline (não instala)

O botão **Preparar credenciais offline (não instala)** e a operação
`--stage-oss-secrets` existem apenas na prévia técnica MIT com payload
ZIP validado. Requerem Administrador/UAC, auditoria dos binários
PostgreSQL e a fase `DATA_DIRECTORIES_ONLY`, com pastas vazias em
`C:\ProgramData\Esterium`. Serviços de produção preexistentes são recusados.

O EXE gera seis credenciais CSPRNG independentes (48 bytes aleatórios
por credencial): `pg-super`, `pg-service`, `pg-app`, `garnet-password`,
`jwt` e `metrics`. Usa arquivos `CreateNew` protegidos para SYSTEM e
Administradores, sem imprimir valores nem colocá-los nos argumentos.
Só depois de escrever e conferir os seis arquivos o diário transita
para `OFFLINE_SECRETS_ONLY`, mantendo banco, serviços e backups
explicitamente NÃO configurados. Não há regeneração automática.

A suíte [40445069719](https://github.com/GustavoMFDS/SistemaERP/actions/runs/40445069719)
confirmou PostgreSQL 16 em ProgramData, identidade CI não administradora,
SQL, backup/restauração, persistência após parada/início do mesmo
serviço SCM e seis segredos distintos preservados após reexecução
rejeitada. TLS/SCRAM, migração, Garnet AOF/TLS e registro parado do
ERP também passaram; o gate foi aprovado. Isso não prova reboot
completo do Windows nem instalação persistente de loja.

## Prova adicional — credencial offline e identidade Windows (somente CI)

O script `test-oss-offline-service-identity-ci.ps1` exige Windows
descartável hospedado no GitHub e as três variáveis explícitas de
segurança do teste integrado. Após a etapa 5, lê a credencial
`pg-service.secret` já protegida, cria uma única conta local
temporária não administradora e usa essa credencial por PSCredential
(nunca por argumento CLI) para autenticar `whoami.exe`. Verifica
SID correspondente e hashes intactos do diário e do segredo;
remove a conta e os arquivos temporários. Não cria serviços de
clientes nem realiza initdb adicional.

Essa prova nova somente poderá ser marcada como aprovada quando
o job integrado e o gate terminarem em `success` no respectivo commit.
O botão `Instalar` permanece bloqueado na prévia MIT; a rota assinada
comercial `-Apply` segue inalterada. Reboot real, contas permanentes,
TLS LAN, backup externo, homologação fiscal e direitos de redistribuição
continuam pendentes. **Manter PR Draft, sem merge ou release.**
