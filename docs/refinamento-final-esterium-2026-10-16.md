# Refinamento final do Esterium ERP — 16/10/2026

**Escopo:** backup automático e restauração (2), alertas no Início (3),
conferência de obrigações fiscais no Fiscal (4), relatórios de operação no Financeiro (5).
Nenhum destes recursos liga a transmissão da SEFAZ ou substitui autenticação/RBAC.

## Backup automático (operação do servidor)

O script `scripts/postgres-daily-backup.sh` fornece backup lógico PostgreSQL
com `pg_dump` em formato custom, validação `pg_restore --list`, checksum
SHA-256, lock exclusivo, arquivos inicialmente temporários e retenção (padrão
14 dias). Ele **não** cria automaticamente um agendamento no seu servidor:
a equipe de infraestrutura deve instalar um cron/job scheduler no mesmo
ambiente que tem acesso autorizado ao banco. O backup e o status devem
ficar **fora do repositório** e fora da pasta pública do servidor.

Requer `PGHOST`, `PGUSER`, `PGDATABASE`, `PGPORT` opcional,
`PGPASSFILE` privado (não expor senha na linha de comando), `BACKUP_DIR`
**absoluto**. Em produção, definir TLS de PostgreSQL adequadamente
(`PGSSLMODE=verify-full`, certificados confiáveis) e restringir permissão
de leitura ao processo de backup. Não colocar credenciais no script ou no
agendador, não copiar dumps para GitHub ou logs de CI.

Exemplo ilustrativo em host Linux administrado (não executar sem revisar
caminhos, SSL, usuário e política de armazenamento):

```bash
export PGHOST=localhost PGPORT=5432 PGUSER=esterium_backup PGDATABASE=esterium
export PGPASSFILE=/etc/esterium/pgpass
export BACKUP_DIR=/srv/esterium-private/backups
export BACKUP_STATUS_PATH=/srv/esterium-private/backups/backup-status.json
export BACKUP_RETENTION_DAYS=14
bash scripts/postgres-daily-backup.sh
```

Exemplo de agendamento: instalar job cron diário com as variáveis acima
via arquivo root/usuário de serviço protegido, e alertar sobre
código de saída não zero. Definir também destino **fora do host**,
criptografia e política de restauração/acesso antes de produção.
**Se executar somente no disco do servidor, não protege contra a
perda do disco físico.**

Para a área administrativa Início ler o estado, disponibilizar
`ESTERIUM_BACKUP_STATUS_PATH` ao processo da API apontando para
o **mesmo arquivo** de status, em modo somente leitura.
A rota `/api/v1/audit/backup-status` exige `audit:read` e não
fornece caminhos, nomes de bancos ou conteúdo das cópias.
Sem configuração, mostra uma pendência em vez de sinal verde.
O estado `ok` prova apenas que o dump foi gerado, listado e
teve checksum verificado; **não prova que possa ser restaurado**.

## Backup no Windows (Task Scheduler / servidor Windows)

Também existe `scripts/postgres-daily-backup.ps1`, equivalente para
Windows. Ele usa `pg_dump.exe`, `pg_restore.exe`, SHA-256,
lock exclusivo de arquivo e limpeza após sucesso. A rotina aceita
`-BackupDir`, `-PostgresBin` e `-RetentionDays`; fornece o mesmo
`backup-status.json` consumido pela Central de Atenção.

```powershell
# Exemplo de operação assistida; ajustar para o servidor protegido.
$env:PGHOST='127.0.0.1'
$env:PGPORT='5432'
$env:PGUSER='esterium_backup'
$env:PGDATABASE='esterium'
$env:PGPASSFILE='C:\ProgramData\Esterium\secrets\pgpass.conf'
.\scripts\postgres-daily-backup.ps1 -BackupDir 'D:\EsteriumBackups'
```

Para automação, configurar o **Agendador de Tarefas do Windows**
com uma conta de serviço de acesso mínimo, variáveis do PostgreSQL
protegidas e execução regular da chamada acima. Configurar repetição,
tratamento de falhas e cópia criptografada fora do computador.
O arquivo `ESTERIUM_BACKUP_STATUS_PATH` deve ser montado/leído pela
API em somente leitura. **Nenhuma tarefa foi registrada automaticamente
na máquina ou em produção por este commit; o agendamento é uma etapa
de implantação**.

**Ensaio local de 16/10/2026:** no Windows de demonstração
`sistemaerp_demo`, o PowerShell gerou um arquivo `esterium-*.dump`
com SHA-256 e `backup-status.json` indicando `ok`.
Uma restauração isolada de backup da mesma base foi realizada em
`esterium_restore_validation_20261016`: 65 tabelas e contagens de
empresas, usuários, produtos, vendas, saldos, notas, obrigações
fiscais e variantes **idênticas** às da origem. O ambiente demo
não possui a tabela `schema_migrations`, portanto a checagem foi
baseada nas tabelas reais, sem inventar uma versão persistida.
Esses testes **não** exercitam desastre com disco perdido, backup
off-site ou restauração de empresas verdadeiras.

## Ensaio de restauração (obrigatório antes do piloto)

Usar `scripts/postgres-backup-drill.sh` com
`SOURCE_DATABASE_URL`, `RESTORE_DATABASE_URL` (banco separado,
descartável e com nome contendo restore/backup/drill/test/validation),
`ALLOW_RESTORE_RESET=1` e `BACKUP_DIR` protegido.
**ATENÇÃO:** esse script apaga o schema public do BANCO DE TESTE
informado; não fornecer a ele banco com dados reais que devam ser
preservados. A verificação inclui versão e estado do schema,
tabelas críticas, contagens representativas e hash.
Não executar o drill automaticamente contra bancos de produção ou
homologação sem plano operacional explícito.

Um ensaio com dados sintéticos demonstra viabilidade técnica, não
recuperação segura e completa de todos os dados de uma loja real.
Definir RPO/RTO com o responsável de cada empresa, medir os tempos de
backup e restore, simular falha de host e registrar provas de
recuperação com storage externo antes do piloto oficial.

## Central de atenção (Início)

Com base em permissões de acesso, consulta:
- `GET /api/v1/fiscal/obligations?unresolved=true&limit=1`:
  contagem total de pendências, **não apenas a página exibida**;
- `GET /api/v1/finance/accounts?kind=all&limit=1`:
  saldo vencido da empresa, sem presumir confirmação bancária;
- `GET /api/v1/inventory/low-stock?limit=1`: total de SKUs
  em atenção de estoque;
- `GET /api/v1/audit/backup-status` para administradores com
  `audit:read`: execução de backup global, aviso de falha/atraso/
  monitoramento ausente.

Falhas de qualquer fonte são exibidas como consulta incompleta,
nunca traduzidas em "nenhuma pendência". Fila offline no navegador
permanece contextual ao PDV daquele dispositivo.

## Conferência fiscal diária (Fiscal)

`GET /api/v1/fiscal/daily-close?date=AAAA-MM-DD` (exige
`invoice:read`). Contagem no banco **sem paginação artificial**,
filtrada pelo tenant autenticado e pela data local brasileira de
criação da obrigação. Distingue NFC-e com protocolo/XML processado
com integridade, pendências, itens para revisão, vendas canceladas,
problemas de arquivo e obrigações de NF-e 55.

Esta é uma **conferência operacional, não um fechamento fiscal legal
nem uma consulta online SEF/MG**. Não autoriza nota, cancela ou
retransmite por conta própria. A exportação CSV é um resumo local,
não substitui XML legal, DANFE ou controles do contador.
Autorizações de NF-e 55 não são afirmadas como comprovadas porque
o emissor modelo 55 ainda não está concluído.

## Relatórios operacionais (Financeiro)

`GET /api/v1/finance/operational-reports?from=AAAA-MM-DD&to=AAAA-MM-DD`
(exige `finance:read`, período até 93 dias), com números da empresa:
- vendas por horário de finalização, no fuso `America/Sao_Paulo`;
- dez produtos principais por subtotal dos itens e margem **bruta
  estimada**, sem descontos gerais, devoluções, impostos ou despesas;
- total de SKUs ativos sem venda finalizada no período e exemplos;
- caixas fechados com diferença no período;
- número atual de contas vencidas, independente do intervalo.

**Não** usar estes números para balancete, DRE ou impostos.
Todas as queries têm `tenant_id` obrigatório; respostas de custo e
margem só estão expostas a contas com `finance:read`.

## Gates para avançar

1. Backend Go test/vet, frontend build/lint e testes negativos de RBAC.
2. Banco real com duas empresas: as quatro novas consultas devem
   mostrar apenas dados da empresa autenticada.
3. Testar dia sem vendas, saldo zero, devoluções, venda cancelada,
   erro de rede e status fiscal inconsistente.
4. Rodar backup + restore em banco descartável e revalidar os hashes,
   migrações e contagens. Repetir o teste em infraestrutura da loja.
5. Piloto com responsáveis reais, impressora e contador, mantendo
   envio fiscal de produção desabilitado até homologação específica.

**PR #25 e PR #15 continuam sem merge.**
