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
