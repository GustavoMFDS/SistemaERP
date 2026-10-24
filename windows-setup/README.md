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
