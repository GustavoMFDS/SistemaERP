# Esterium Local — entrada técnica unificada para primeira instalação

**Estado: pré-lançamento.** A entrada `scripts/install-esterium-local-complete.ps1`
foi criada para encadear, em um procedimento técnico único, as duas etapas
**já existentes** de primeira instalação. Não é instalador gráfico para
consumidores nem prova de que EDB.exe / NSSM funcionem em todas as máquinas.

## Pré-requisitos de uma distribuição aprovada

- Windows x64 novo, com privilégios de instalação; não exige Go nem Docker
  no PC da loja.
- Release do ERP obtida de fonte aprovada, verificada por manifesto SHA-256
  **independentemente conhecido** e por assinatura Authenticode de editor
  aprovado. Sem certificado real aprovado, **não executar `-Apply`**.
- Instalador EDB PostgreSQL 16 e assinatura do fornecedor, Garnet ZIP e
  serviço NSSM, todos com hash SHA-256 aprovado **fora do próprio ZIP**.
- Origem HTTPS do navegador preparada e validada independentemente, sem
  usar uma conexão HTTP na LAN. A tela do cliente não é liberada nesta etapa.
- Instalação obrigatoriamente nova: pastas, contas, serviços e portas livres.
  Uma máquina com instalação ou banco pré-existente deve seguir procedimento
  de atualização/recuperação, não o instalador inicial.

## Procedimento do operador técnico

A distribuição assinada deve fornecer os valores aprovados para:

```powershell
$setup = @{
  SourceBundleDir = 'C:\Install\EsteriumApproved'
  AllowedOrigin = 'https://erp.loja.exemplo'
  PostgresInstaller = 'C:\Install\Vendor\postgresql-16-windows-x64.exe'
  ExpectedPostgresSHA256 = '<SHA256_EDB_APROVADO_EXTERNO>'
  ExpectedPostgresPublisherThumbprint = '<CERTIFICADO_EDB_APROVADO>'
  GarnetPackage = 'C:\Install\Vendor\garnet.zip'
  ExpectedGarnetSHA256 = '<SHA256_GARNET_APROVADO_EXTERNO>'
  ServiceWrapperExe = 'C:\Install\Vendor\nssm.exe'
  ExpectedServiceWrapperSHA256 = '<SHA256_NSSM_APROVADO_EXTERNO>'
  ApprovedManifestSHA256 = '<SHA256_MANIFESTO_RELEASE_APROVADO_EXTERNO>'
  TrustedPublisherThumbprint = '<EDITOR_ESTERIUM_APROVADO>'
}
# 1. Plano read-only de requisitos: nao autentica o editor comercial.
& 'C:\Install\EsteriumApproved\tools\install-esterium-local-complete.ps1' @setup
# 2. Plano read-only COM manifesto e editor genuinamente aprovados fora do ZIP.
& 'C:\Install\EsteriumApproved\tools\install-esterium-local-complete.ps1' @setup -VerifyApprovedRelease
# 3. -Apply somente apos aceite, com pacote real assinado e plano aprovado:
# & 'C:\Install\EsteriumApproved\tools\install-esterium-local-complete.ps1' @setup -Apply
```

Valores entre `<...>` são **marcadores**, não hashes/certificados reais.
A assinatura deve ser feita antes de gerar/validar o manifesto final,
seguindo a cadeia de confiança do projeto.

O `-VerifyApprovedRelease` executa uma verificação mais rigorosa e
sem alterações no Windows: autentica primeiro os scripts que verificam a
release, confere o SHA-256 do manifesto obtido por canal independente e
exige assinatura Authenticode **válida e confiável** de todos os executáveis
e scripts privilegiados. O `-Apply` exige exatamente a mesma conferência
**antes de qualquer alteração**. Um PLAN comum não aprova o editor
comercial, e certificado fictício de CI não habilita instalação.

O procedimento faz a pré-verificação **das duas etapas** antes de qualquer
escrita; apenas no modo `-Apply` validado executa, na ordem:
1. `setup-esterium-local-dependencies.ps1`: vendor PostgreSQL 16 e Garnet,
   configuração offline de credenciais/TLS, serviços desabilitados/parados;
2. `finish-esterium-local-first-install.ps1`: staging protegido, banco
   novo e esquema versionado, segredos da aplicação, serviço ERP em modo
   parado e prova de dependências, restaurando estados parados/desabilitados.

**Falhas são intencionalmente bloqueantes.** Se a primeira fase falhar,
não se executa a segunda. Uma falha após a criação do cluster/banco não
autoriza repetir `-Apply`, tentar downgrade SQL, apagar dados ou ligar ERP:
a instalação parcial deve ser examinada por um técnico.

## Testes, escopo e aceite ainda pendente

O CI valida sintaxe, rejeição de assinatura e origem/portas inseguras,
conteúdo obrigatório do bundle e provas sintéticas/independentes de
PostgreSQL/Garnet. **Não comprova** a execução completa do EDB EXE ou
NSSM autênticos em um Windows novo com certificado comercial.

Para liberar instalação real faltam: instalação em VM/PC limpo com
distribuição assinada, validação HTTPS dos clientes, restauração de backup
em outro computador, testes simultâneos de caixas LAN, recuperação após
queda e aceite explícito do checklist [#151](https://github.com/GustavoMFDS/SistemaERP/issues/151).

Após a primeira instalação, a liberação/start automático continua
deliberadamente **bloqueada** até passar pelos gates de operação.
