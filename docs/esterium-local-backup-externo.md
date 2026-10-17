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

## Escopo do teste automatizado

O teste `scripts/test-verified-external-backup.ps1` copia **somente bytes sintéticos**, verifica hash e arquivo de checksum, impede sobrescrita e rejeita arquivo vazio. Ele roda no CI Windows e não precisa de um PostgreSQL real.

O teste **não prova** que um compartilhamento SMB é outro equipamento, nem autenticação, criptografia, desconexão da rede, falha de energia ou recuperação completa de uma base PostgreSQL com as migrações e obrigações fiscais existentes.

## Antes de liberar uma loja

- Testar backup real e restauração em **outro computador** e banco descartável, comparando tabelas, registros e integridade fiscal.
- Garantir criptografia em repouso no destino externo, controle de acesso mínimo, proteção contra ransomware e retenção independente.
- Configurar alertas quando a cópia externa falhar ou quando a última restauração ensaiada ficar antiga.
- Ensaiar reinício do serviço e queda de energia durante backup e durante transações simultâneas de PDV.
- Nenhuma liberação de emissão SEFAZ antes de credenciamento, homologação e protocolo genuíno.
