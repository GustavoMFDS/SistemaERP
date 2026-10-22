# Esterium Local — atualização, retorno e recuperação (Windows)

**Status: procedimento técnico em desenvolvimento, sem liberação comercial.**
Não executar com dados de uma loja até existir pacote assinado, editor
independentemente aprovado e aceite do Windows físico. As ferramentas desta
página não autorizam emissão fiscal real.

## O que está disponível

| Fluxo | Ferramenta | Limite obrigatório |
| --- | --- | --- |
| Conferir atualização | `plan-esterium-windows-upgrade.ps1` | Somente leitura; exige serviço parado, backup local e cópia em outro computador |
| Atualizar somente aplicativo | `apply-esterium-app-update.ps1` | NÃO aplica migrações SQL; preserva versão antiga em pasta de rollback |
| Voltar à versão anterior | `restore-esterium-app-version.ps1` | Somente aplicativo, SQL idêntico e backup recente verificado |
| Remover serviço Windows | `unregister-esterium-windows-service.ps1` | Remove somente o registro SCM, nunca arquivos ou dados |
| Verificar backup | `check-esterium-backup-integrity.ps1` | Compara SHA-256 local/externo; NÃO prova que o arquivo é restaurável |
| Exercitar restauração | `ci-postgres-cross-instance-restore.sh` | CI de PostgreSQL descartável; NÃO substitui restore real em outro PC |

## Regras de segurança antes de qualquer atualização

1. Confirmar que **todas as vendas pendentes** foram sincronizadas/revisadas e
   os caixas encerrados. Bloquear acesso dos demais terminais durante manutenção.
2. Parar o serviço EsteriumERP e desabilitar temporariamente a tarefa de backup
   agendado. Nenhum comando deste guia faz isso automaticamente.
3. Ter **backup PostgreSQL recente** e uma cópia em compartilhamento SMB de
   **outro computador**, com verificação SHA-256 positiva e estado documentado.
   A recuperação tem de ser ensaiada com banco isolado antes de um piloto real.
4. Aprovar por canal independente o SHA-256 do manifesto e a impressão digital
   do certificado **real** do editor Authenticode. O ZIP técnico do CI não está
   assinado para distribuição e é **rejeitado** pelo fluxo de produção.
5. Confirmar que o pacote não contém migrações SQL novas ou modificadas.
   O rollback desta página **não volta uma migração do banco**. Caso contrário,
   exigir plano específico, compatibilidade de esquema e ensaio de recuperação;
   o atualizador de aplicativo deve recusar a operação.

## Como conferir e recuperar uma atualização apenas de aplicativo

Os comandos abaixo são **modelos administrativos**. Substitua os caminhos
por diretórios reais, e os valores aprovados por informações recebidas
separadamente do pacote. Execute o script assinado de uma área administrativa
confiável **fora** da pasta de instalação que será substituída.

Primeiro, após uma atualização, preserve a linha `ROLLBACK FOLDER:` exibida
pelo atualizador. Ela identifica a cópia íntegra anterior, sob Program Files.
Não renomeie nem mova essa pasta para outro local.

```powershell
$parameters = @{
    InstalledDir = 'C:\Program Files\EsteriumERP'
    PreviousVersionDir = 'C:\Program Files\.esterium-update-rollback-<ID-REAL>'
    BackupDir = 'D:\BackupEsterium'
    ExternalBackupDir = '\\servidor-backup\esterium'
    ApprovedManifestSHA256 = '<HASH-64-DA-VERSAO-ANTERIOR-APROVADO>'
    TrustedPublisherThumbprint = '<IDENTIDADE-40-DO-EDITOR-APROVADA>'
}
# Só confere o plano. Não grava nem inicia serviço ou banco.
.\restore-esterium-app-version.ps1 @parameters
```

Apenas após inspeção do plano, aprovação independente e autorização explícita
do técnico responsável, o **mesmo script assinado** permite aplicar:

```powershell
.\restore-esterium-app-version.ps1 @parameters -Apply -ConfirmRollback
```

O procedimento verifica que a origem é uma pasta real
`.esterium-update-rollback-<GUID>` irmã da instalação. Depois delega ao
atualizador protegido: confere assinaturas e hashes, serviço parado,
cópia externa, igualdade exata dos arquivos de migração, prepara os arquivos,
verifica novamente, substitui o diretório e guarda também o aplicativo que
estava instalado antes do retorno. **Não inicia automaticamente o serviço.**

Se falhar, **não repita cegamente** nem apague a pasta de rollback.
Registre logs técnicos sem senhas, separe a máquina da operação e faça
diagnóstico de integridade. Nunca desative validações para forçar o retorno.

## Desinstalação conservadora (sem destruir dados)

A ação disponível **não é uma desinstalação completa**.
O script `unregister-esterium-windows-service.ps1`, com serviço parado,
identidade correta e backup agendado desabilitado, retira somente o registro
do serviço após aprovação administrativa e da assinatura. Por design:

- **Preserva** PostgreSQL, Garnet, arquivos de dados, fotos, backups, segredos,
  certificados, logs e migrações, além dos executáveis e do agendamento.
- Não apaga diretórios do ProgramData nem remove serviços de fornecedores.
- O responsável deve inventariar e guardar os arquivos e registrar quem os
  poderá restaurar. Uma remoção integral comercial requer fluxo separado,
  comprovação de retenção legal/fiscal e confirmação específica de qualquer
  exclusão. Não automatizar exclusão por padrão.

## Conferir o backup no computador de recuperação (somente leitura)

A nova ferramenta `check-esterium-restore-archive.ps1` inspeciona um arquivo
`.dump` **em um segundo Windows**, antes de iniciar qualquer restauração.
É necessário possuir o backup e seu arquivo `.sha256`, mais **dois SHA-256
aprovados por fonte independente**: o do backup e o do binário
`pg_restore.exe` do PostgreSQL 16 instalado em
`C:\Program Files\PostgreSQL\16\bin`.

Exemplo técnico sem dados reais (substituir os valores ilustrativos por
aprovações genuínas, e executar somente a ferramenta assinada da release):

```powershell
.\check-esterium-restore-archive.ps1 `
    -ArchivePath 'D:\Recuperacao\esterium-AAAAmmddTHHMMSSZ-1234abcd.dump' `
    -ExpectedArchiveSHA256 '<SHA256-64-DO-ARQUIVO-APROVADO>' `
    -PgRestoreExe 'C:\Program Files\PostgreSQL\16\bin\pg_restore.exe' `
    -ExpectedPgRestoreSHA256 '<SHA256-64-DA-FERRAMENTA-APROVADA>'
```

O comando recusa links para fora do diretório, arquivos alterados, divergência
da soma de verificação, binários PostgreSQL não aprovados, versão diferente da
16, formato não reconhecido, backups sem tabelas comerciais críticas e arquivos que contêm apenas o esquema das tabelas, sem entradas de dados.
Ele executa **apenas** `pg_restore --version` e `pg_restore --list`: não
usa senha, não abre conexão, não cria banco e não altera arquivos do cliente.

**Limite fundamental:** listar um arquivo `pg_dump` não comprova que todas
as linhas são restauráveis. Mesmo com resultado positivo, o responsável deve
criar **uma nova instância isolada em outro computador**, restaurar nela
com `pg_restore --exit-on-error`, comparar o estado das migrações e os
dados de empresas, produtos, vendas e registros fiscais, e registrar a
evidência do ensaio antes de liberar operação. O CI tem ensaios de
restauração entre instâncias independentes somente com dados fictícios;
não equivale a essa validação física.

## Aceite de recuperação em computador independente

Antes de liberar o piloto, registrar evidências de um exercício em Windows
separado: pacote original, versão do PostgreSQL, backup externo e hash esperado,
restauração isolada, contagem conferida de tabelas/vendas e documentos, fotos
restauradas, acesso segregado por loja, consistência após reinício e ausência
de duplicação. O restore deve ocorrer em **banco descartável**; nunca testar
restauração por cima de banco produtivo. Testes de CI com dados sintéticos
são necessários, mas não substituem a evidência desse exercício físico.

**Situação atual:** as ferramentas permitem preparar e verificar cenários
de atualização/retorno sem tocar no banco. Não há release Authenticode comercial,
procedimento de migração SQL automaticamente reversível, garantia de restauração
real independente ou desinstalador completo aprovado para loja.
