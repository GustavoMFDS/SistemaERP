# Identidade técnica e trilha imutável de inspeção fiscal — 11/10/2026

Este incremento do PR #25 avança a issue #32 sem antecipar emissão fiscal ou
atribuir ações automáticas a funcionários.

## Registro de identidade da instância

A migração 0043 cria `fiscal_worker_identities` para processos locais de
pré-verificação fiscal. O nome de cada instância é gerado aleatoriamente
no início do aplicativo, não corresponde a um usuário humano e não concede
permissões administrativas, acesso a certificados ou capacidade de transmitir.

O registro tem UUID próprio, criação, renovação, prazo de expiração e
revogação. Cada execução da rotina renova sua identidade antes de solicitar
trabalho. Um processo expirado ou revogado não pode reassumir um job usando
o mesmo registro; reinícios criam identidades novas.

## Trilha auditável e imutabilidade

A tabela `fiscal_preflight_audit_events` registra, por CNPJ/tenant e venda:

- `claimed`: qual instância assumiu um token de lease, e se recuperou
  um token que havia expirado;
- `completed`: qual instância concluiu a inspeção e o código de resultado.

O registro ocorre **no mesmo UPDATE e na mesma transação PostgreSQL** que
altera o lease, usando gatilho. Falha ao identificar uma instância válida
impede a concessão ou conclusão: *fail-closed*.

O banco bloqueia UPDATE e DELETE da trilha e conserva FKs para impedir
apagar evidências com jobs existentes. Não registrar certificados, chaves
privadas, payload SOAP nem tokens de lease em logs da API.

## Consulta pelo gerente

`GET /api/v1/fiscal/obligations/{saleID}/preflight-audit?limit=20`

- Exige autenticação e permissão `invoice:read`; filtro rígido por
  `tenant_id` obtido do usuário autenticado.
- Valida UUID da venda; retorna 404 quando ela não pertence à empresa.
- Retorna até 100 eventos, ordenados do mais recente ao mais antigo.
- Não retorna token secreto de lease, nem finge atribuição a um usuário.
- O painel Fiscal mostra `Histórico técnico` por venda, incluindo
  identidade curta do processo, horário e retomadas; mensagens avisam
  que isso não representa autorização pela SEFAZ.

## Verificações locais executadas

- PostgreSQL 16 temporário: **43/43 migrações e dados de teste** aprovados.
- Teste integrado `TestFiscalDispatchClassifiesWithoutIssuing` aprovado:
  instância não registrada não assume trabalho, múltiplas instâncias
  não assumem o mesmo lease, recuperação de lease expirado rejeita token
  antigo, instância revogada não pode renovar, eventos são gravados
  com UUIDs técnicos e a mutação posterior do histórico é rejeitada.
- `go test ./...`, `go vet ./...`, build frontend e
  Playwright Chromium painel fiscal/PDV aprovados (2/2); lint 0 erros,
  12 avisos já existentes.
- Nenhum documento `invoices` criado, reservado ou transmitido por
  esta rotina. A trilha **não** equivale a prova de autorização fiscal.

## Pendências e limites

O preflight continua sendo diagnóstico local e **não** um executor fiscal
real. Faltam habilitação controlada por empresa, identidade operacional
específica para assinatura e transmissão, classificação tributária por
UF/operação, NFC-e/NF-e modelos 65/55, cálculo IBS/CBS, pagamentos,
homologação com certificado A1 e protocolo verdadeiro da SEFAZ,
recuperação após timeout de rede e proteção de envio contra duplicidade.

O mecanismo de imutabilidade protege alterações ordinárias de
aplicação; controles adicionais de menor privilégio para a conexão
PostgreSQL e retenção legal devem ser definidos para produção.

**Flag de produção e homologação remota permanecem desabilitadas.**
PR continua em rascunho, sem merge e sem GitHub Actions.
