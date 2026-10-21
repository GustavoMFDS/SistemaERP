# Instalação offline PostgreSQL + Garnet — etapa técnica Windows

**Status: em revisão, não liberada para loja.** O comando tools/install-esterium-local-dependencies.ps1 executa o instalador EDB PostgreSQL 16 em modo silencioso e extrai/registra Garnet como serviço Windows, a partir de arquivos obtidos e aprovados previamente. Ainda não é um Setup.exe único: a preparação do banco e as migrações já têm um coordenador técnico assinado de segunda fase, mas o fluxo real do instalador EDB/NSSM ainda precisa de homologação.

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

O coordenador também gera, em pasta nova protegida, modelos `postgresql.conf` e `pg_hba.conf` que impõem somente escuta local, TLS mínimo 1.2 e autenticação SCRAM-SHA-256. O pacote inclui `esterium-pg-tls-certgen.exe`, gerador local em Go (somente biblioteca padrão), que emite `certificates/server.crt`, `certificates/server.key` (RSA 3072 bits) e `certificates/root.crt` (CA local). A chave privada da CA é descartada; a chave privada do servidor só recebe leitura para SYSTEM, Administradores e a conta Windows dedicada do PostgreSQL; `root.crt` concede leitura ao serviço LocalService do ERP. Nenhum desses segredos ou certificados entra no ZIP distribuído. Um verificador estático confere as regras de TLS/SCRAM. **A política TLS e os modelos são preparados e aplicados offline pelo novo coordenador**, sem iniciar o PostgreSQL para uso comercial. A CI verifica com PostgreSQL real `sslmode=verify-full`, `sslrootcert`, HBA/SCRAM e recuperação; ainda falta testar o mesmo processo usando o instalador EDB EXE real no Windows descartável. O certificado do servidor expira em dez meses; renovar com procedimento assinado e testado, não sobrescrever a identidade em uso.

O pacote contém também `initialize-esterium-postgres-app-db.ps1`, inicializador técnico para **um cluster novo já aprovado em TLS**. O gerador de bootstrap cria `pg-app.secret` independente das senhas de administrador e serviço Windows. Após verificar `sslmode=verify-full`, HBA, SCRAM e ausência de conta/banco anteriores, o inicializador cria `esterium_app` sem superusuário, sem permissão de criar contas ou bancos e um banco `esterium`, testa o login e grava `database-url.secret` (usado por `DATABASE_URL_FILE`) com leitura exclusiva por LocalService, Administradores e SYSTEM. As senhas não entram na linha de comando. **O comando não executa migrações nem é chamado pelo instalador enquanto PostgreSQL estiver desabilitado.** Uma falha após criar a conta ou o banco exige recuperação manual e nunca permite repetição automática sobre um banco existente.

A etapa `apply-esterium-initial-schema.ps1` utiliza `esterium-migrate.exe` para executar, na ordem, as migrações `.up.sql` somente se o banco `esterium` estiver **novo e sem tabelas**. O comando recusa arquivos de migração sem par ou fora da sequência; em produção, exige manifesto SHA-256 e assinaturas aprovados do pacote inteiro. A URL do processo filho não contém senha: o segredo de `DATABASE_URL_FILE` é transferido apenas ao ambiente restrito do migrador e a conexão continua em `sslmode=verify-full`. O resultado precisa indicar a última versão sem `dirty`; em caso de falha, não há repetição nem `force` automática. **As migrações ainda não são chamadas no fluxo de instalação até a configuração TLS, inicialização da conta, backup e recuperação estarem aprovados.**

Para a confiança TLS do Garnet, o bootstrap agora exporta `garnet-server.cer` **somente com a parte pública** do mesmo certificado usado pelo serviço. O arquivo fica legível por LocalService (ERP), SYSTEM e Administradores, sem acesso do serviço Garnet à confiança do cliente. O backend aceita `REDIS_CA_CERT_FILE`, que deve apontar para esse arquivo DER (ou PEM) e exige `rediss://`, verificação do nome `localhost`, TLS 1.2+ e validação de certificado normal, sem `InsecureSkipVerify`. O registrador do serviço Windows aceita `-RedisCACertFile` e grava apenas o caminho protegido no ambiente SCM. **Nenhuma raiz é instalada no repositório global de confiança do Windows.** O fluxo automático ainda precisa reunir o registro do serviço, sua ativação e a validação das operações reais do cache.

A configuração do instalador EDB exige agora `enable_acledit=1`, opção documentada pela EDB para conceder à conta Windows dedicada acesso ao caminho do diretório de dados. O verificador offline recusa `enable_acledit=0` ou qualquer alteração inesperada do arquivo protegido. **A permissão efetiva e a instalação do EDB `.exe` ainda precisam de smoke real em Windows descartável antes da liberação comercial.**

No fluxo novo, após o instalador EDB registrar e deixar **desabilitado/parado** o PostgreSQL 16, o próprio instalador grava um comprovante restrito em `PostgresInstallOptions/PostgresFresh-<serviço>.marker`. Somente com esse comprovante, conta Windows dedicada, diretório de cluster `PG_VERSION=16` e porta fechada, `stage-esterium-postgres-tls.ps1` substitui *offline* `postgresql.conf` e `pg_hba.conf` pelos modelos locais TLS/SCRAM. Cada configuração original EDB é preservada com backup próprio, e um recibo de aplicação bloqueia reaplicações sobre a mesma instalação. O helper exige assinatura Authenticode aprovada fora da CI. **Nenhum serviço é iniciado ou habilitado por essa etapa**; uma falha parcial exige diagnóstico e recuperação, nunca reinstalação automática sobre dados existentes. O teste sintético Windows usa apenas diretório descartável, sem modificar um banco real.

A nova etapa técnica `probe-esterium-local-first-run.ps1` aproxima as dependências da ativação final, mas **não as libera**. Ela exige o pacote Windows instalado e assinado, registro prévio do serviço `EsteriumERP` em modo Manual/Stopped, arquivos TLS e senhas com permissões restritas, recibo de instalação inicial do PostgreSQL e Garnet configurado para AOF e recuperação. No modo padrão apenas verifica e informa o plano. Com `-Apply` e hash SHA-256/assinatura previamente aprovados, muda PostgreSQL e Garnet temporariamente para Manual, verifica que suas portas pertencem aos serviços Windows corretos e roda `esterium-server.exe --check-dependencies` usando somente os caminhos de arquivos protegidos registrados para o ERP. **Durante a prova, o serviço do ERP também é desabilitado para impedir inicialização externa. **Mesmo após sucesso os dois serviços de dependência são parados e desabilitados novamente, enquanto o ERP volta a Manual/Stopped, sem atender vendas.** O resultado positivo produz um único `FirstRunReadiness/probe-result.json` com os hashes do pacote/configurações, ACL de Administradores/SYSTEM e sinalizadores `automaticStartupApproved=false`, `backupRestoreApproved=false` e `fiscalEmissionApproved=false`. Em erro, o ERP permanece desabilitado e nenhum recibo é gerado.** Uma falha também aciona o desligamento e desabilitação, sem limpar dados SQL. A validação do executável EDB, backup/restauração independente e aceite de ativação automática continuam pendentes. O teste CI cobre assinatura obrigatória, análise sintática e contratos de falha; ainda falta teste integrado do ciclo SCM com EDB e Garnet reais.

A CI do PostgreSQL agora possui também um teste separado de **restauração lógica**: depois de criar as 51 migrações, gravar uma linha sintética e forçar uma parada abrupta, executa `pg_dump` em formato customizado, faz uma cópia com `copy-verified-external-backup.ps1` e SHA-256 em outra pasta **sintética no mesmo runner**, e restaura a partir **dessa cópia** com `pg_restore --single-transaction` em um segundo banco descartável e valida versão do schema e linha confirmada. Essa verificação é diferente da recuperação do log transacional (WAL), mas ainda **não** prova restauração de um backup externo da loja, proteção contra falhas de disco, nem procedimento de recuperação operável pelo cliente. A ativação automática continua bloqueada até esses critérios adicionais.

Novo coordenador SQL técnico `bootstrap-esterium-local-schema.ps1`: após a criação e configuração TLS do PostgreSQL, mas **antes** da prova integrada dos serviços, exige um pacote já instalado em Program Files com assinatura aprovada, um PostgreSQL 16 novo em `Disabled/Stopped`, comprovante de configuração TLS/SCRAM e os dois arquivos de senha protegidos da instalação. `-Apply` inicia **somente o PostgreSQL**, chama os inicializadores já testados de conta/base e das 51 migrações versionadas, e encerra/desabilita novamente o banco mesmo em falha. Não executa o ERP, não ativa Garnet, não transmite documentos fiscais e não corrige automaticamente esquemas parciais. Se a criação de banco falhar depois de começar, a intervenção é manual; repetir `-Apply` é proibido. Há testes Windows de análise sintática e assinatura; a execução completa deste coordenador com instalador EDB ainda precisa de validação real em máquina descartável.

Ordem tecnica viavel (ainda nao automatizada em Setup.exe): (1) validar a distribuicao assinada e executar stage-esterium-approved-windows-release.ps1 para instalar uma copia protegida em Program Files/EsteriumApprovedSetup; (2) executar setup-esterium-local-dependencies.ps1 para gerar segredos, instalar PostgreSQL/Garnet desligados e preparar TLS; (3) executar bootstrap-esterium-local-schema.ps1 DA COPIA PROTEGIDA para abrir o PostgreSQL temporariamente, criar a conta/base SQL e aplicar as 51 migracoes, encerrando e desabilitando o servico; (4) gerar JWT e metricas com provision-esterium-app-secrets.ps1; (5) registrar EsteriumERP como Manual/Stopped via register-esterium-windows-service.ps1, usando como origem a copia protegida e como credenciais os arquivos criados, incluindo database-url.secret; (6) rodar probe-esterium-local-first-run.ps1 a partir do pacote final instalado, para conferir banco/cache sem abrir vendas e com desligamento posterior. Isso resolve a dependencia circular anterior: registro do ERP exige DATABASE_URL_FILE, mas criacao da base depende de executaveis assinados e aprovados em Program Files. Nao ativar automaticamente antes do teste real do EXE da EDB, backup externo restaurado, acesso seguro via HTTPS/local e aceite fisico.

Foi adicionado o coordenador tecnico finish-esterium-local-first-install.ps1 para a segunda fase (depois de setup-esterium-local-dependencies.ps1). Com um unico comando assinado e argumentos de release previamente aprovados, ele encadeia staging protegido do pacote, bootstrap SQL completo, geracao JWT/metricas, registro parado do servico EsteriumERP e probe real das dependencias. Sua preparacao exige um AllowedOrigin HTTPS externo ja configurado: o fluxo de navegador local sem dominio/certificado proprio ainda nao foi resolvido e NAO deve receber liberaçao comercial. O modo padrao so planeja e valida a distribuicao; em falha, uma instalacao parcial requer avaliacao manual, sem repetir migrações ou apagar dados. O ERP e os servicos de dependencias nao sao habilitados para iniciar automaticamente. Testes CI verificam o parser, assinatura obrigatoria, recusa de origem insegura e presenca de todas as etapas, mas nao substituem um teste completo do EDB EXE e NSSM em Windows descartavel.

**Ainda não é instalador de loja:** o comando instala serviços desabilitados e parados, mas não instala confiança TLS, não cria banco SQL/usuários comerciais, não executa migrações e não inicia o ERP. Em falha parcial, mantém dados e pastas para inspeção; repetir cegamente a instalação é bloqueado.

## Fluxo para laboratório Windows x64 descartável

1. Aprovar independentemente SHA-256 do EDB PostgreSQL 16 (EXE), Garnet (ZIP) e NSSM (EXE). Verificar Authenticode EDB e a procedência do ZIP Garnet fora dos próprios pacotes. Nunca aceitar hashes enviados pelo pacote a instalar.
2. Gerar os cinco arquivos de bootstrap protegidos automaticamente com o novo provision-esterium-dependency-bootstrap.ps1 (planejamento por padrão; -Apply somente assinado/admin). Produz duas senhas PostgreSQL, ACL Garnet com conta padrão desativada, PFX localhost, senha do PFX e arquivo rediss:// específico do serviço Esterium sob leitura exclusiva de LocalService. A geração não instala a confiança TLS no cliente.
3. Executar prepare-esterium-local-dependency-configs.ps1 em plano e depois em -Apply assinado, usando os cinco arquivos gerados em ProgramData\Esterium\DependencyBootstrap. Ele cria o arquivo de opções EDB e o garnet.conf sob ProgramData/Esterium, com ACLs restritas.
4. Executar install-esterium-local-dependencies.ps1 SEM -Apply: confere assinaturas/hashes, ZIP, ACLs, serviço, portas, diretórios e opções EDB sem instalar nada.
5. Somente com pacote/editor aprovado, executar -Apply elevado: roda EDB unattended sem pop-up, verifica cluster 16, serviço e escuta local, extrai Garnet e registra via NSSM com conta NETWORK SERVICE, início desabilitado (até homologação) e sem iniciar Garnet. Em erro posterior ao registro do Garnet, tenta desfazer somente o serviço criado agora.
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
- O arquivo redis-url.secret é criado automaticamente com URL rediss://localhost, usuário Garnet dedicado e senha aleatória. Somente LocalService, SYSTEM e Administradores podem ler o arquivo; ele pode ser passado ao registro do serviço Esterium como REDIS_URL_FILE. Isso **não** desativa a verificação do certificado TLS: o emissor de confiança ainda precisa ser aprovado no Windows antes do uso.
- O gerador de TLS do PostgreSQL não instala raízes na lista de CAs do Windows, não copia certificados de um computador para outro e não emite uma chave CA reutilizável. Como a configuração comercial ainda não está ativa, **não habilitar o serviço PostgreSQL antes de verificar `sslmode=verify-full` e o `pg_hba_file_rules` efetivo.**
- O NSSM tem hash independente obrigatório, e deve ter licenciamento/proveniência revisados. Não pode ser movido após registro do serviço. Servidor Garnet permanece desabilitado e parado até validações TLS e funcionalidade Redis/Lua.
- O Garnet agora recebe EnableAOF, WaitForCommit e CommitFrequencyMs=0, com Recover e FailOnRecoveryError habilitados; os dados ficam em uma pasta state gravável somente pelo serviço, separada do JSON de configuração. Isto é um **perfil conservador bloqueado para primeira ativação**: não iniciar o serviço até comprovar bootstrap inicial, recuperação e integridade do AOF com binários reais. Uma configuração com Recover=true pode rejeitar um diretório ainda vazio; o instalador não ignora nem contorna esse bloqueio.
- Também é necessário definir capacidade de armazenamento, backup da pasta state e procedimento de redução segura do AOF. A confirmação de gravação aumenta a latência; apenas testes reais com falha forçada aprovam o perfil.
- CI somente exercita planejamento sintético no Windows descartável; **nunca executa os binários de terceiros** nem demonstra instalação/upgrade comercial.
- O bootstrap de senhas/ACL, PFX localhost, REDIS_URL_FILE, confiança TLS local do Garnet, política PostgreSQL, conta/base SQL e 51 migrações já têm helpers e coordenadores assinados. A CI exercita PostgreSQL e Garnet reais separadamente, inclusive recuperação de AOF/WAL e restauração SQL descartável. Permanecem: teste integrado do EDB EXE com NSSM no Windows, fluxo seguro de navegador local/HTTPS, backup externo restaurado em ambiente independente, UI Setup.exe assinada e procedimentos homologados de recuperação/desinstalação. Os gates de distribuição assinada do projeto continuam obrigatórios.
