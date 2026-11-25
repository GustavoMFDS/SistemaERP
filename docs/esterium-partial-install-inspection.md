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

## Retomada restrita do primeiro administrador (candidata)

A inspeção assinada na VM parcial retornou ADMIN_ABSENT e os três serviços voltaram a Stopped/Disabled. Isso autoriza planejar a retomada, não uma ativação automática. A causa específica do erro histórico continua desconhecida.

A nova mídia VM-LAB inclui, além do wrapper e CLI, o helper assinado bootstrap-esterium-first-admin.ps1 e seu digest separado. Conferir artefato, proveniência, assinaturas e todos os hashes contra recibos independentes. Usar diretório diagnóstico protegido e manter o pacote ERP original intacto. Preservar o checkpoint frio 91dc8407; executar PLAN incluindo BootstrapScriptSHA256 e confirmar serviços e portas. Somente após aprovação do PLAN, optar explicitamente por ResumeAdmin (não combinar com Inspect). A própria operação revalida a ausência do admin antes de chamar o helper; Create revalida o estado fresco e recusa tabelas comerciais não vazias na transação serializável original. Não se pede senha em chat, argumentos ou arquivos: operador digita duas vezes no prompt protegido do convidado. O helper envia UTF-8 explícito pelo pipe e limpa o buffer de bytes. O CLI mantém a validação definitiva de caracteres e bytes.

A operação exige ADMIN_COMMITTED após a transação e verifica o desligamento dos três serviços antes de publicar sucesso. Em erro, códigos fixos distinguem validações conhecidas, ausência não confirmada, bootstrap recusado e commit não confirmado; mensagens não reconhecidas não são reproduzidas. Não repetir automaticamente após erro: executar Inspect e revisar o estado. Mesmo ADMIN_COMMITTED não permite criar recibos artificiais, liberar portas, iniciar ERP/Garnet ou ativar HTTPS. A retomada de probe e ativação será um gate posterior separado. Não apagar ou reinstalar banco. A correção de UTF-8 elimina uma fragilidade potencial; não prova que a codificação causou a falha original.

Validação local: AST de quatro scripts, diff check, onze cenários simulados e transporte real de bytes UTF-8 sintéticos pelo pipe Windows. CI completo, assinatura da nova mídia e execução real ResumeAdmin ainda pendentes.

## Continuação da ativação HTTPS — candidata exclusiva da VM atual

O coordenador `resume-esterium-partial-activation.ps1` é separado da retomada do administrador. Somente após ADMIN_COMMITTED confirmado, obter o artefato assinado e conferir a proveniência, certificado e hashes de todos os arquivos, inclusive coordenador e inspector. Usar diretório protegido e preservar snapshot frio. Os pins de TLS devem ser conferidos independentemente contra os arquivos originais de laboratório, sem divulgar hash ou conteúdo da chave no Git/chat; nenhuma troca de certificado ou edição de configuração é autorizada. O endereço 10.0.2.15, origem https://10.0.2.15:8443, listener 0.0.0.0:8443 e CIDR 10.0.2.15/32 são fixos para esta VM. Confirmar NAT sem encaminhamentos no host e ausência de operadores concorrentes. Verificar validade do certificado HTTPS e confiança no convidado antes do Apply. Não usar em outra instalação.

Executar PLAN (sem Activate) com pins originais e diagnósticos, incluindo InspectorScriptSHA256 e TLSCertSHA256/TLSKeySHA256. O PLAN não executa consulta SQL nem prova todos os gates do probe original. Somente após revisão, optar por Activate: o inspector assinado revalida ADMIN_COMMITTED, faz cleanup e deixa os três serviços desabilitados. Coordenador muda apenas ERP para Manual, chama o probe original assinado instalado, exige recibo legítimo e dependências desabilitadas, então chama ativação original assinada. Exige serviços Running/Auto e recibo de ativação. Qualquer falha durante a ativação exige tentativa de desligar/desabilitar todos os serviços e confirmar portas livres; falha de cleanup exige isolamento e revisão manual. Preservar quaisquer recibos genuínos/remanescentes, não apagar ou fabricar para repetir. HTTPS real, regra de firewall e recibos são responsabilidade dos delegates originais; verificação funcional, reboot e backup são gates posteriores. Sem emissão fiscal ou autorização comercial.

Validação local: AST, diff check, 7 cenários simulados de admin ausente, probe/recibo, HTTPS/recibo e cleanup. CI remoto, pacote assinado e execução real deste coordenador ainda pendentes; fixture completa EDB/ZIP normal não equivale a testar a retomada do estado parcial.
