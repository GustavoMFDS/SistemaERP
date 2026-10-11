# Acompanhamento persistente de emissão fiscal — 11/10/2026

## Objetivo e limite deliberado

Cada venda cria uma obrigação fiscal por trigger PostgreSQL da migração
0039. A migração **0041** adiciona um registro de acompanhamento
`fiscal_dispatch_jobs` por (tenant_id, sale_id), criado no **mesmo commit**
da obrigação, com índice para verificação de pendências.

O processo `StartFiscalDispatchReconciliation` percorre as obrigações
automaticamente na inicialização e depois uma vez por minuto. A consulta
usa `FOR UPDATE SKIP LOCKED` e atualizações atômicas limitadas a 100
registros/lote para funcionar com várias instâncias. Status e momento da
última verificação ficam persistidos no banco. A tela fiscal exibe uma
explicação em linguagem simples do estágio atual, sem expor informações
de outras empresas.

**Este serviço é exclusivamente de acompanhamento local**: não cria
reserva de números, não assina XML, não retransmite documentos, não
consulta nem envia para a SEFAZ e não substitui um executor fiscal real.
Uma referência de certificado configurada significa somente que há
dados para serem examinados, NÃO que o certificado, a operação fiscal,
as tabelas tributárias ou a SEFAZ estejam prontos para uso.

### Estados

| Estado | Significado |
| --- | --- |
| pending | Obrigação criada, aguardando primeira triagem |
| waiting_sale | Venda ainda não finalizada |
| awaiting_configuration | Falta referência básica de certificado/configuração |
| awaiting_fiscal_processing | Configuração referenciada; emissão ainda não iniciada |
| reserved, xml_generated, signed | Existe artefato técnico em estágio intermediário |
| awaiting_sefaz | Envio já registrado; resultado requer reconciliação segura |
| authorized | Nota com protocolo, chave, data e nfeProc arquivado, para modelo 65 |
| missing_processed_xml | ALERTA: status autorizado sem arquivo processado associado |
| rejected_review | Rejeição a revisar |
| cancelled_review | Venda/documento cancelado, exige revisão |
| manual_review | Histórico anterior à obrigatoriedade, não processar automaticamente |
| unsupported_model | NF-e 55 ainda sem processo suportado |
| requires_review | Estado ou modelo inesperado, falha fechada |

A classificação da nota autorizada exige `invoices.status='authorized'`,
modelo 65, protocolo, data e relação com o XML processado de 0040. A API
real de download verifica também o hash SHA-256 armazenado. A classificação
não é evidência independente de aceitação pela SEFAZ.

### Evidências de teste

- PostgreSQL 16 temporário: **41 migrações + seed**, sucesso.
- Teste integrado `TestFiscalDispatchClassifiesWithoutIssuing`: venda cria
  obrigação e job na mesma transação; sem configuração, classifica
  `awaiting_configuration`; com referência do certificado, classifica
  `awaiting_fiscal_processing` **sem gerar invoices**; tipo `nfe`
  passa para `unsupported_model` sem emitir indevidamente.
- `go test ./...`, `go vet ./...` e build do frontend aprovados;
  lint sem erros (12 avisos preexistentes); Playwright Chromium
  fiscal e impressão **2/2 aprovados**.
- O teste exige `TEST_FISCAL_DISPOSABLE_DB=1` e banco exclusivo. Os
  testes de numeração/cancelamento do PR precisam também de bancos
  descartáveis devido a gatilhos de imutabilidade fiscal.

### Próximas condições de aceitação antes de transmissão

1. Definir executor com identidade técnica auditável, leasing distribuído
   para chamadas remotas, reprocessamento/retry e bloqueio por empresa/UF.
2. Selecionar corretamente NFC-e 65 versus NF-e 55 conforme operação e
   regras da UF; não presumir que toda venda é modelo 65.
3. Validar dados fiscais reais e operações por UF, incluindo tributos
   IBS/CBS, CFOP, CST, cClassTrib, pagamentos e identificação do cliente.
4. Usar certificado A1 e endpoints SEFAZ de homologação configurados,
   validar schema e assinatura, consultar situação após timeout antes de
   reenviar, persistir protocolo genuíno e nfeProc atômicos.
5. Cobrir idempotência/reinícios/concorrência, eventos de cancelamento e
   contingência, impressora e distribuição de documentos.

**As flags de transmissão à SEFAZ continuam desligadas.** Este
incremento não encerra as issues #32 (emissão automática) nem #33
(arquivo autorizado/homologação). Não houve merge.
