# Instalação offline PostgreSQL + Garnet — etapa técnica Windows

**Status: em revisão, não liberada para loja.** O comando tools/install-esterium-local-dependencies.ps1 executa o instalador EDB PostgreSQL 16 em modo silencioso e extrai/registra Garnet como serviço Windows, a partir de arquivos obtidos e aprovados previamente. Ainda não é um Setup.exe único nem prepara banco comercial automaticamente.

## Fluxo para laboratório Windows x64 descartável

1. Aprovar independentemente SHA-256 do EDB PostgreSQL 16 (EXE), Garnet (ZIP) e NSSM (EXE). Verificar Authenticode EDB e a procedência do ZIP Garnet fora dos próprios pacotes. Nunca aceitar hashes enviados pelo pacote a instalar.
2. Gerar os cinco arquivos de bootstrap protegidos: duas senhas PostgreSQL, ACL Garnet com usuário default desabilitado, PFX Garnet e senha do PFX. Ver detalhes em esterium-local-service-registration.md.
3. Executar prepare-esterium-local-dependency-configs.ps1 em plano e depois em -Apply assinado. Ele cria o arquivo de opções EDB e o garnet.conf sob ProgramData/Esterium, com ACLs restritas.
4. Executar install-esterium-local-dependencies.ps1 SEM -Apply: confere assinaturas/hashes, ZIP, ACLs, serviço, portas, diretórios e opções EDB sem instalar nada.
5. Somente com pacote/editor aprovado, executar -Apply elevado: roda EDB unattended sem pop-up, verifica cluster 16, serviço e escuta local, extrai Garnet e registra via NSSM com conta NETWORK SERVICE, início manual e sem iniciar Garnet. Em erro posterior ao registro do Garnet, tenta desfazer somente o serviço criado agora.
6. Completar TLS verify-full PostgreSQL, SCRAM e pg_hba.conf, certificado/chave, usuário e banco SQL mínimos, migrações e testes reais Garnet TLS/ACL/Lua. Conferir serviço e PID com check-esterium-local-dependencies.ps1 -RequireServiceOwnership. Provar backup externo e restauração independente antes de dados comerciais.

Exemplo somente leitura, com placeholders (não reutilizar valores do exemplo em produção):

~~~powershell
$common = @{
  PostgresInstaller = 'D:\PacotesAprovados\postgresql-16-windows-x64.exe'
  ExpectedPostgresSHA256 = '<SHA256-64-aprovado-externamente>'
  ExpectedPostgresPublisherThumbprint = '<THUMBPRINT-40-EDB>'
  GarnetPackage = 'D:\PacotesAprovados\garnet-windows-x64.zip'
  ExpectedGarnetSHA256 = '<SHA256-64-aprovado-externamente>'
  ServiceWrapperExe = 'D:\PacotesAprovados\nssm.exe'
  ExpectedServiceWrapperSHA256 = '<SHA256-64-aprovado-externamente>'
  PostgresOptionsFile = 'C:\ProgramData\Esterium\PostgresInstallOptions\postgresql-unattended.options'
  GarnetConfigFile = 'C:\ProgramData\Esterium\GarnetConfig\garnet.conf'
}
.\tools\install-esterium-local-dependencies.ps1 @common
# A execução efetiva também exige:
# -TrustedPublisherThumbprint '<EDITOR-ESTERIUM-40>' -Apply
~~~

## Segurança e limitações

- O instalador recusa serviços, pastas e portas já usados. Nunca limpa bancos preexistentes e não força reinstalação.
- A opção EDB contém credenciais em texto pois o fornecedor assim requer; é restrita a SYSTEM/Administradores e excluída após a tentativa de instalação, inclusive em falha. Os arquivos-fonte continuam protegidos. Exclusão não garante eliminação forense em SSD.
- Falhas após criação do serviço PostgreSQL preservam o cluster e a conta para diagnóstico. Após falha ao registrar Garnet, remove apenas o serviço criado na mesma invocação; binários e configurações ficam para análise.
- O NSSM tem hash independente obrigatório, e deve ter licenciamento/proveniência revisados. Não pode ser movido após registro do serviço. Servidor Garnet permanece parado até validações TLS e funcionalidade Redis/Lua.
- CI somente exercita planejamento sintético no Windows descartável; **nunca executa os binários de terceiros** nem demonstra instalação/upgrade comercial.
- Ainda faltam autogeração segura de certificados/segredos, contas e banco SQL, migrações automáticas, UI assinada de instalação/atualização, smoke efetivo EDB/NSSM/Garnet e procedimentos homologados de recuperação/desinstalação. Os gates de distribuição assinada do projeto continuam obrigatórios.
