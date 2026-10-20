# Instalação offline PostgreSQL + Garnet — etapa técnica Windows

**Status: em revisão, não liberada para loja.** O comando tools/install-esterium-local-dependencies.ps1 executa o instalador EDB PostgreSQL 16 em modo silencioso e extrai/registra Garnet como serviço Windows, a partir de arquivos obtidos e aprovados previamente. Ainda não é um Setup.exe único nem prepara banco comercial automaticamente.

## Instalação em um comando técnico

Agora o pacote contém também tools/setup-esterium-local-dependencies.ps1. Por padrão apenas valida pacotes offline, hashes aprovados, conflitos de serviços e portas e pastas de destino. No modo -Apply, exige administrador, assinatura Authenticode do launcher e auxiliares, além de manifesto de release aprovado independentemente. Em sequência, cria senhas/ACL/PFX protegidos, gera os arquivos de configuração e executa os binários offline de PostgreSQL e Garnet.

Exemplo de plano somente leitura, usando placeholders que devem ser substituídos por aprovações externas:

~~~powershell
$payload = @{
  PostgresInstaller = 'D:\PacotesAprovados\postgresql-16-windows-x64.exe'
  ExpectedPostgresSHA256 = '<SHA256-64-aprovado-externamente>'
  ExpectedPostgresPublisherThumbprint = '<THUMBPRINT-40-EDB>'
  GarnetPackage = 'D:\PacotesAprovados\garnet-windows-x64.zip'
  ExpectedGarnetSHA256 = '<SHA256-64-aprovado-externamente>'
  ServiceWrapperExe = 'D:\PacotesAprovados\nssm.exe'
  ExpectedServiceWrapperSHA256 = '<SHA256-64-aprovado-externamente>'
}
.\tools\setup-esterium-local-dependencies.ps1 @payload
# -Apply exige também ApprovedManifestSHA256 e TrustedPublisherThumbprint.
~~~

**Ainda não é instalador de loja:** o comando instala serviços em estado manual/parado, mas não instala confiança TLS, não cria banco SQL/usuários comerciais, não executa migrações e não inicia o ERP. Em falha parcial, mantém dados e pastas para inspeção; repetir cegamente a instalação é bloqueado.

## Fluxo para laboratório Windows x64 descartável

1. Aprovar independentemente SHA-256 do EDB PostgreSQL 16 (EXE), Garnet (ZIP) e NSSM (EXE). Verificar Authenticode EDB e a procedência do ZIP Garnet fora dos próprios pacotes. Nunca aceitar hashes enviados pelo pacote a instalar.
2. Gerar os cinco arquivos de bootstrap protegidos automaticamente com o novo provision-esterium-dependency-bootstrap.ps1 (planejamento por padrão; -Apply somente assinado/admin). Produz duas senhas PostgreSQL, ACL Garnet com conta desativada por padrão e usuário específico, PFX localhost e sua senha. A geração não instala a confiança TLS no cliente.
3. Executar prepare-esterium-local-dependency-configs.ps1 em plano e depois em -Apply assinado, usando os cinco arquivos gerados em ProgramData\Esterium\DependencyBootstrap. Ele cria o arquivo de opções EDB e o garnet.conf sob ProgramData/Esterium, com ACLs restritas.
4. Executar install-esterium-local-dependencies.ps1 SEM -Apply: confere assinaturas/hashes, ZIP, ACLs, serviço, portas, diretórios e opções EDB sem instalar nada.
5. Somente com pacote/editor aprovado, executar -Apply elevado: roda EDB unattended sem pop-up, verifica cluster 16, serviço e escuta local, extrai Garnet e registra via NSSM com conta NETWORK SERVICE, início manual e sem iniciar Garnet. Em erro posterior ao registro do Garnet, tenta desfazer somente o serviço criado agora.
6. Completar TLS verify-full PostgreSQL, SCRAM e pg_hba.conf, certificado/chave, usuário e banco SQL mínimos, migrações e testes reais Garnet TLS/ACL/Lua. Conferir serviço e PID com check-esterium-local-dependencies.ps1 -RequireServiceOwnership. Provar backup externo e restauração independente antes de dados comerciais.

Exemplo de preparação automática de credenciais, com plano sem escrita e execução posterior restrita a release autenticada:

~~~powershell
.\tools\provision-esterium-dependency-bootstrap.ps1
# Somente depois de conferir origem e assinatura do editor Esterium:
# .\tools\provision-esterium-dependency-bootstrap.ps1 -Apply -TrustedPublisherThumbprint '<EDITOR-ESTERIUM-40>'
~~~

Exemplo de planejamento da instalação de binários, com placeholders (não reutilizar valores do exemplo em produção):

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
- O Garnet agora recebe EnableAOF, WaitForCommit e CommitFrequencyMs=0, com Recover e FailOnRecoveryError habilitados; os dados ficam em uma pasta state gravável somente pelo serviço, separada do JSON de configuração. Isto é um **perfil conservador bloqueado para primeira ativação**: não iniciar o serviço até comprovar bootstrap inicial, recuperação e integridade do AOF com binários reais. Uma configuração com Recover=true pode rejeitar um diretório ainda vazio; o instalador não ignora nem contorna esse bloqueio.
- Também é necessário definir capacidade de armazenamento, backup da pasta state e procedimento de redução segura do AOF. A confirmação de gravação aumenta a latência; apenas testes reais com falha forçada aprovam o perfil.
- CI somente exercita planejamento sintético no Windows descartável; **nunca executa os binários de terceiros** nem demonstra instalação/upgrade comercial.
- O bootstrap de senhas/ACL e PFX localhost foi automatizado, mas ainda faltam instalação da cadeia de confiança TLS para clientes, certificado e política PostgreSQL, contas/banco SQL, migrações automáticas, UI assinada de instalação/atualização, smoke efetivo EDB/NSSM/Garnet e procedimentos homologados de recuperação/desinstalação. Os gates de distribuição assinada do projeto continuam obrigatórios.
