# Esterium Local — backup externo verificado no Windows

Este marco expande o backup PostgreSQL já existente, sem mudar migrações, vendas ou emissão fiscal. **Backup no mesmo computador não basta** se o SSD da loja falhar.

## Agendamento com cópia externa obrigatória

O script de backup `scripts/postgres-daily-backup.ps1` aceita agora `-ExternalBackupDir` e `-RequireExternalCopy`. Em operação de loja, configure os dois parâmetros apontando para uma **pasta de rede SMB de outra máquina**; a conta de serviço Windows precisa ter acesso ao compartilhamento.

Exemplo conceitual, sem senha na linha de comando:

```powershell
.\scripts\postgres-daily-backup.ps1 -BackupDir 'D:\Esterium\Backups' -ExternalBackupDir '\\servidor-de-backup\esterium' -RequireExternalCopy
```

Os dados de conexão PostgreSQL e credenciais da rede devem ficar em gerenciamento seguro da conta de serviço, como `PGPASSFILE` com ACL restrita; nunca embutir senha em arquivos distribuídos, atalhos ou argumentos. O agendamento deve ser registrado no Agendador de Tarefas do Windows sob conta com as permissões mínimas.

O backup é marcado como válido **somente** se o arquivo PostgreSQL local for íntegro e, com destino externo configurado, a cópia remota também for confirmada por SHA-256. O arquivo é primeiro copiado como temporário, validado, renomeado e recebe `.sha256`; nunca substitui uma cópia já existente. Uma falha no destino remoto mantém o backup local, porém registra o estado como falha para atenção do administrador.

O JSON `backup-status.json` informa `archive_verified` e `external_copy_verified` sem revelar caminho de rede ou credenciais. A limpeza por prazo de retenção continua apenas na pasta local; a retenção, a criptografia e o controle de acesso do destino remoto devem ser configurados separadamente.

## Conferência diária de integridade e atualização

O `backup-status.json` passa a incluir também `archive_name` e `archive_sha256`, mas **nunca** senhas, caminhos de compartilhamento ou dados comerciais. Em caso de falha, os dois campos são esvaziados.

O pacote Windows inclui `tools/check-esterium-backup-integrity.ps1`, um verificador **somente leitura** que exige status `ok`, arquivo local e cópia externa da **mesma geração**, SHA-256 correspondente e arquivos `.sha256` válidos, além de um horário dentro do prazo configurado. Arquivos faltantes, adulterados ou backup antigo geram erro para o operador. O verificador exige compartilhamento de rede UNC; a exceção para pastas locais só funciona nos testes sintéticos do runner Windows hospedado.

Exemplo, depois de configurar o compartilhamento e a conta de serviço (sem credenciais na linha de comando):

```powershell
.\tools\check-esterium-backup-integrity.ps1 `
  -BackupDir 'D:\Esterium\Backups' `
  -ExternalBackupDir '\\servidor-de-backup\esterium' `
  -MaxAgeHours 30
```

O erro da verificação pode ser utilizado por um monitor de tarefas local, mas **o CI não registra notificações nem agendamentos na loja**. Para a operação real, ainda é obrigatório executar restaurações ensaiadas em outro computador, proteger a cópia externa por ACL/criptografia e monitorar as falhas.

## Tarefa diária Windows protegida (estágio piloto)

O pacote passa a conter as ferramentas run-esterium-scheduled-backup.ps1 e
register-esterium-backup-task.ps1 em tools/. O registro usa uma configuração
JSON **protegida, fora de Program Files e do ZIP**, com os campos obrigatórios
PGHOST, PGPORT, PGUSER, PGDATABASE, PGPASSFILE, PGSSLROOTCERT, PostgresBin,
BackupDir, ExternalBackupDir e RetentionDays. Nenhum valor de senha pode
existir nesse JSON nem nos argumentos do Agendador; PGPASSFILE aponta para
um arquivo secreto separado. Os dois arquivos exigem ACL de leitura explícita
para NT AUTHORITY\NETWORK SERVICE, **sem permissão de modificar, excluir ou alterar ACL**;
somente SYSTEM e Administradores podem administrá-los. Qualquer outra conta ou
grupo autorizado é rejeitado, inclusive Users/Everyone/Authenticated Users.

O executor recusa banco fora de loopback, exige TLS PostgreSQL com
PGSSLMODE=verify-full, ferramentas PostgreSQL existentes, arquivo de senha
protegido, e destino externo SMB em outra máquina. O certificado
`PGSSLROOTCERT` também deve ser um arquivo físico, sem link simbólico nem
permissão de escrita/exclusão/alteração de acesso para contas que não sejam
SYSTEM ou Administradores. Como a CA não contém segredo, somente a **leitura**
por outros usuários pode ser permitida; a capacidade de substituir a CA não. A execução faz backup,
cópia remota obrigatória e verificação SHA-256/frescor. Configurações inválidas
falham sem fazer dump; falhas do backup não são convertidas em sucesso.

Para uma conferência sem contato com o banco, SMB ou Agendador, após
provisionar arquivos de configuração e segredos restritos no Windows:

    .\tools\run-esterium-scheduled-backup.ps1 -ConfigFile 'C:\ProgramData\Esterium\Backup\backup-config.json' -CheckOnly
    .\tools\register-esterium-backup-task.ps1 -ConfigFile 'C:\ProgramData\Esterium\Backup\backup-config.json'

O segundo comando é apenas um **plano** por padrão. O registro real requer
`-Apply`, administrador, **SHA-256 do manifesto aprovado por canal independente**
(`-ApprovedManifestSHA256`) e impressão digital do certificado do editor já
aprovado (`-TrustedPublisherThumbprint`). Antes de registrar, exige assinatura
Authenticode válida do launcher e dos auxiliares, além de validar **todo o
pacote instalado** pelo gate de distribuição. Não reutilize um hash ou editor
informado pelo próprio ZIP não confiável como se fosse aprovação. A tarefa
`EsteriumERP-Backup` anterior deve estar ausente.

Cria a tarefa diária com identidade NetworkService, **desabilitada por padrão**
até verificação presencial e autorização explícita de habilitação. A conta da
máquina deve ter direito de escrita no compartilhamento remoto: não presumir
que uma credencial de usuário logado seja herdada pelo serviço.

O teste automatizado de registro real do Agendador usa uma exceção
`-UnsignedDisposableCIFixture` **somente** no runner descartável e identificado
do GitHub Actions, com dados sintéticos e tarefa desabilitada, removida no
encerramento. Essa opção **não** é uma alternativa à assinatura para uma loja.

O administrador deve demonstrar previamente permissões, execução e
**restauração em outro PC**, além de monitorar falhas; esta ferramenta
**não** cria usuários, senhas, certificados, tarefas de alerta ou proteção
contra ransomware. CI usa somente arquivos sintéticos e verifica a rejeição
de senha inline, banco remoto, destino local e ACL insegura.

## Escopo do teste automatizado

### Conferir duas gerações independentes de backup

Para um aceite mais rigoroso, depois de existirem **duas execuções diárias** com arquivos diferentes, o verificador `tools/check-esterium-backup-integrity.ps1` aceita `-MinimumVerifiedCopies 2`. Além do último backup válido, exige outro arquivo de data anterior (até sete dias), disponível tanto na pasta local quanto na cópia externa e conferido novamente por SHA-256 e seus arquivos auxiliares. Se a segunda cópia estiver ausente, alterada ou incompleta, a verificação reprova.

Essa opção não altera o banco, não cria arquivos, não elimina backups e não substitui a restauração ensaiada em outra máquina. A rotina automática continua com `MinimumVerifiedCopies=1` na primeira execução para não falhar artificialmente quando ainda não existem dois dias de arquivos. O CI exercita esse reforço usando **somente dados sintéticos**.

O teste `scripts/test-verified-external-backup.ps1` copia **somente bytes sintéticos**, verifica hash e arquivo de checksum, impede sobrescrita e rejeita arquivo vazio. Ele roda no CI Windows e não precisa de um PostgreSQL real.

O teste **não prova** que um compartilhamento SMB é outro equipamento, nem autenticação, criptografia, desconexão da rede, falha de energia ou recuperação completa de uma base PostgreSQL com as migrações e obrigações fiscais existentes.

## Antes de liberar uma loja

- Testar backup real e restauração em **outro computador** e banco descartável, comparando tabelas, registros e integridade fiscal.
- Garantir criptografia em repouso no destino externo, controle de acesso mínimo, proteção contra ransomware e retenção independente.
- Configurar alertas quando a cópia externa falhar ou quando a última restauração ensaiada ficar antiga.
- Ensaiar reinício do serviço e queda de energia durante backup e durante transações simultâneas de PDV.
- Nenhuma liberação de emissão SEFAZ antes de credenciamento, homologação e protocolo genuíno.


### Provisionamento da identidade PostgreSQL (etapa de staging, #239)

O arquivo `scripts/stage-esterium-postgres-backup-role.sql` prepara somente
a identidade PostgreSQL `esterium_backup`, inicialmente **NOLOGIN**, sem
senha. Não é migração do ERP nem deve ser executado automaticamente durante
instalação. Exige revisão/aprovação do arquivo pelo responsável técnico e
sessão PostgreSQL administrativa local com TLS `verify-full`. Executar
com psql em modo `-X -v ON_ERROR_STOP=1` e a base `esterium`, somente após
conferir a identidade do servidor e o estado da instalação.

A transação recusa base ou proprietário inesperados, migração suja, conta
preexistente, RLS e privilégios efetivos excessivos no schema public;
concede SELECT nas tabelas/sequências atuais e privilégios padrão SELECT
para objetos futuros do proprietário da aplicação e do administrador
executor. Não altera contas existentes nem retira permissões de PUBLIC.
O executor agendado exige exatamente `PGUSER=esterium_backup` e
`PGDATABASE=esterium`, recusando o uso acidental de contas administrativas
ou da aplicação. A validação do nome não comprova, sozinha, os privilégios
reais do papel na base.
A configuração de transação read-only é defesa adicional, não substitui
as permissões. Outros schemas, extensões, funções executáveis e mudanças
posteriores de privilégios exigem revisão de cobertura antes do piloto.

**Ainda não habilita login, não provisiona senha/PGPASSFILE ou ACL Windows,
não registra tarefa e não comprova acesso SMB como NetworkService.**
A ativação exige procedimento local aprovado, credencial própria e
verificação de acesso efetivo; não reutilizar credencial da aplicação.
Em workgroup, uma sessão SMB interativa não prova a autorização da
identidade de serviço. Não habilitar o agendamento até teste nessa
identidade e restauração comprovada.

A fixture `scripts/ci-esterium-backup-role.sh` exercita staging,
recusas, rollback, privilégios efetivos, objetos futuros e pg_dump com
SET ROLE em PostgreSQL descartável do job integration. Não é prova de
autenticação real da conta ou backup externo na instalação Windows.


### Proposta de configuração sem escrita

O novo `scripts/plan-esterium-backup-config.ps1` produz somente o JSON
de configuração (dez campos, sem valores de senha), com identidade e
banco dedicados e caminhos locais separados dos arquivos de backup.
Requer caminhos explícitos para PGPASSFILE, CA e destino SMB, sem ler os
arquivos ou contactar a rede. Rejeita destinos evidentemente locais,
caminhos relativos e arquivos secretos dentro do diretório de arquivos
de backup. Não comprova que um nome SMB corresponde a outro equipamento.

O planejador integra a lista de ferramentas do próximo bundle Windows;
a distribuição assinada ainda depende da validação e aprovação da release. Sua saída não é um arquivo protegido: não redirecionar
para um diretório público ou substituir configuração existente. O passo
seguinte é o provisionador local assinado com ACLs e verificação
`-CheckOnly` do executor. O planejador não cria senha, habilita LOGIN,
instala tarefas ou valida autenticação real. Os testes exercitam a
proposta antes de criar o diretório sintético do runner.


### Staging protegido da configuração (implementação em revisão)

`scripts/provision-esterium-backup-config.ps1` usa o planejador e, em
PLAN, não grava arquivos. O Apply exige administrador, execução no
diretório de ferramentas instalado, editor e manifesto aprovados
independentemente e todos os auxiliares assinados. Exige pasta Esterium
já existente com proprietário e permissões de escrita restritos.
Prepara uma pasta temporária protegida, cria JSON com ACL de leitura
NetworkService e controle administrativo, executa o CheckOnly existente
e publica a pasta nova `ProgramData\Esterium\BackupConfig` sem
sobrescrever configuração anterior. A credencial e CA precisam existir
por procedimento independente; o script não lê o conteúdo da senha,
não habilita LOGIN e não toca no Agendador. A pasta de arquivos de backup
não é criada por este componente.

O código e os testes de PLAN/recusa sem aprovação estão em revisão; o
Apply assinado ainda exige ensaio próprio. O provisionador e planejador
integram as listas do manifesto, empacotamento e fixtures. Não usar na
instalação atual; aguardar um candidato assinado e aprovado. Não alterar a política de confiança
para executar esse código em laboratório.


### Preparação local da conta de backup a partir do pacote aprovado

O SQL de staging passa a integrar `tools/stage-esterium-postgres-backup-role.sql`
no manifesto SHA-256 obrigatório. Como SQL não é um executável Authenticode,
sua integridade depende do manifesto aprovado por canal independente,
validado juntamente com a assinatura de todos os executáveis e scripts.
Não executar SQL recebido avulsamente ou de uma branch sem aprovação.

No computador servidor, um administrador PostgreSQL autorizado deve abrir
uma sessão local autenticada pelo procedimento de TI, com TLS verify-full
e a base esterium, e executar o SQL aprovado. O resultado esperado é
uma conta NOLOGIN, sem senha e com permissões limitadas. Se ela já existir,
parar e revisar, sem sobrescrever a identidade. A ativação posterior exige
uma credencial própria configurada pelo operador local sem compartilhá-la
por chat, argumentos ou logs, e uma prova de conexão e dump com o papel
dedicado. Não usar o papel administrativo para produzir o backup. O
provisionador de configuração utiliza um arquivo pgpass previamente
aprovado e protegido; não cria a senha nem altera o papel no banco.

A autorização do compartilhamento externo deve ser comprovada para a
identidade efetiva da tarefa, antes de habilitar o agendamento. Em
workgroup, a sessão SMB do usuário interativo não é evidência disso.

### Verificação da identidade na execução real

Antes do dump, o executor agendado verifica em sessão PostgreSQL real a
identidade de login dedicada, base, TLS, estado read-only padrão, ausência
de memberships e privilégios excessivos, cobertura de leitura no schema
public e ausência de RLS/funções SECURITY DEFINER executáveis. Recusa
qualquer divergência sem iniciar dump. A verificação não substitui revisão
de outros schemas, grants futuros ou teste de restauração. Configurações
libpq herdadas que poderiam redirecionar autenticação ou sessão são
removidas; psql e pg_dump não solicitam senha interativamente. O fixture
CI também verifica a recusa do usuário administrativo pelo SQL exato
do executor. O teste positivo de login TLS na VM permanece pendente.


O teste Windows PostgreSQL TLS existente passa a preparar a conta de
backup na base integralmente migrada, ativar uma credencial exclusivamente
sintética, verificar a consulta de identidade do executor em conexão
SCRAM/TLS real e criar o arquivo de recuperação com a conta dedicada.
A restauração integral já existente usa esse mesmo arquivo, mantendo a
comparação de migração, dados e bytes de imagens. A prova ainda depende
de execução CI e não substitui o ensaio SMB/NetworkService na VM.
