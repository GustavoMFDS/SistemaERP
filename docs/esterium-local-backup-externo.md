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
