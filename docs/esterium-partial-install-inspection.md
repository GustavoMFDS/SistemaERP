# Runbook de inspeção de instalação parcial — VM-LAB

Escopo: somente a VM VirtualBox isolada de homologação com PostgreSQL EDB 16, Garnet e Esterium ERP nos caminhos padrão. Não é recuperação automática, reparo de instalação ativada nem procedimento comercial.

## Pré-condições (gate obrigatório)

- Preservar snapshot/backup frio da instalação parcial e registrar a identidade do checkpoint; confirmar NAT sem redirecionamento de portas e ausência de operadores concorrentes.
- CI verde no commit exato, inclusive fixture SQL real e testes simulados. Usar o artefato VM-LAB desse run, não compilar/assinar localmente como substituto.
- Conferir o recibo histórico do instalador original (manifesto ERP e editor). Conferir separadamente o novo manifesto de recuperação, SHA-256 dos dois arquivos, certificado público, editor e validade. Não aceitar valores autoaprovados pelo executável.
- Instalar confiança temporária apenas no usuário da VM, após conferência explícita do certificado. Nunca instalar raiz de laboratório no host.
- Copiar somente o script e o CLI assinados para novo diretório Program Files\\EsteriumRecoveryDiagnostics, com ACL administrativa (SYSTEM e Administrators como únicos escritores, sem herança permissiva). Não substituir arquivos de Program Files\\EsteriumERP nem editar configurações, banco, credenciais ou serviços.
- Verificar que não há Apply em execução; ERP, Garnet e PostgreSQL registrados, parados e desabilitados; nenhuma evidência de readiness/ativação. Se qualquer pré-condição divergir, parar e revisar.

## Execução

Usar PowerShell elevado no convidado com a política de assinatura vigente. Invocar inspect-esterium-partial-install.ps1 com OriginalManifestSHA256, OriginalPublisher, DiagnosticSHA256 (digest do CLI novo) e DiagnosticPublisher obtidos dos dois recibos independentes. Omitir Inspect na primeira invocação: PLAN não altera serviços. Conferir o resultado e só então invocar o mesmo script com Inspect. Não inserir senha ou URL do banco na linha de comando. O script consulta o arquivo protegido já existente através do CLI assinado.

A inspeção inicia somente o PostgreSQL temporariamente, verifica porta e identidade do listener, executa uma consulta transacional read-only e tenta restaurar todos os serviços a Stopped/Disabled. O PostgreSQL pode escrever seus próprios arquivos de runtime/WAL durante start/stop; não é uma inspeção fisicamente imutável do volume. Não interromper o script durante essa janela. Falha de cleanup exige intervenção manual; não assumir que os serviços estão seguros sem verificar.

## Decisão após evidência

| Resultado | Ação |
|---|---|
| ADMIN_ABSENT | Não repetir Apply; confirmar todos os invariantes de banco novo, causa da falha e projetar um bootstrap assinado de retomada. |
| ADMIN_COMMITTED | Não recriar administrador; revisar configuração original e projetar continuação assinada do probe/ativação, sem inventar recibos. |
| ADMIN_INDETERMINATE | Parar; investigar somente leitura. |
| RECOVERY_CLEANUP_FAILED | Isolar o convidado e revisar o estado dos serviços antes de qualquer outra ação. |

Mesmo ADMIN_COMMITTED não prova login, senha, ausência de dados comerciais, readiness ou aprovação HTTPS. Não iniciar ERP/Garnet ou emitir fiscal, não criar recibos artificiais. Guardar somente os códigos e evidências não sensíveis. Remover a confiança descartável após finalizar o laboratório.

## Estado desta implementação

O wrapper é candidato, testado localmente com serviços simulados; pacote assinado, compatibilidade com instalação original e inspeção real na VM ainda dependem de validação. Não executar em produção.
