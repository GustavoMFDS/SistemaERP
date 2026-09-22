# Offline PDV (Etapa 13)

Objetivo: permitir que o PDV continue operando em quedas de rede **curtas/intermitentes**, registrando vendas localmente e sincronizando depois, sem duplicar lançamentos.

## Escopo (MVP)

- Persistência local: fila de requisições no `localStorage` (browser), isolada por tenant + usuário
- Fila de eventos: cada venda (`POST /api/v1/sales`) é registrada como um item na fila
- Sincronização posterior: ao voltar online, o frontend drena a fila em ordem
- Segurança/consistência: idempotência no backend para retries (`Idempotency-Key`)

## Frontend

- Página: `web/src/pages/PDVPage.tsx`
- Fila: `web/src/lib/offlineQueue.ts`

Fluxo:
1. PDV monta e tenta sincronizar pendências se `navigator.onLine`.
2. Em `Finalizar`, uma única `Idempotency-Key` é gerada antes do primeiro envio.
3. Se o browser estiver offline ou a resposta se perder após o envio, a venda é enfileirada reutilizando exatamente a mesma chave.
4. Evento `online` dispara `flushQueue()` para sincronizar.
5. Rejeições permanentes 4xx ficam preservadas como itens que requerem atenção e não bloqueiam vendas posteriores; falhas transitórias/rede interrompem o flush para retry posterior.

Observação: o `cash_session_id` precisa já existir (caixa aberto previamente). Este MVP não tenta “abrir caixa offline”.

## Backend

- Header suportado: `Idempotency-Key`
- Rota: `POST /api/v1/sales`
- Implementação:
  - `pg_advisory_xact_lock` serializa concorrência por chave
  - tabela `idempotency_keys` guarda o resultado (sale_id + total)

Migração:
- `backend/migrations/0004_idempotency_keys.up.sql`

## Como testar

1. Abrir o PDV online e carregar produtos.
2. Simular offline no browser (DevTools → Network → Offline).
3. Finalizar uma venda: deve aparecer como “registrada offline (pendente sync)”.
4. Voltar online: o PDV sincroniza automaticamente.

## Seguranca e privacidade operacional

- Após 24 horas, itens ainda não sincronizados deixam de ser reenviados automaticamente e passam para estado de atenção; o registro local é preservado para reconciliação.
- A fila guarda somente o payload necessario para recriar a venda, o cabecalho `Idempotency-Key` e metadados mínimos de reconciliação (estado/erro/última tentativa).
- Evite incluir dados pessoais sensiveis no payload de venda offline. Quando o cliente for opcional, prefira venda sem identificacao.
- `localStorage` nao e um cofre criptografico. Nao ha chave segura no frontend para criptografia forte sem apoio do usuario/dispositivo.
- A funcao `clearOfflineQueue()` permite limpeza manual controlada quando o operador precisar descartar pendencias locais.
- Caixa, cache de produtos e fila usam chaves derivadas de `tenant_id + user_id`, impedindo que outro tenant/usuário leia o estado anterior no mesmo navegador. Logout limpa apenas o escopo atual antes de remover o access token.
- A fila legada global `sistemaemgo:offlineQueue:v1` nunca é executada automaticamente. Se detectada após upgrade, o operador pode importá-la explicitamente para revisão; os itens entram em `attention` e exigem retry manual.
- Cabecalhos sensiveis como `Authorization`, cookies e tokens nao sao persistidos na fila.
- O backend usa `request_hash`: mesma chave + mesmo hash reaproveita o resultado; mesma chave + hash diferente retorna `409 conflict`.



## Cenarios E2E obrigatorios

- venda offline normal e sincronizacao ao reconectar;
- resposta perdida depois de o backend processar a venda: retry deve usar a mesma `Idempotency-Key` e nao duplicar a venda;
- dois tenants/usuarios no mesmo browser nao compartilham caixa, cache de produtos ou fila;
- rejeicao permanente de um item nao impede a sincronizacao dos itens posteriores;
- item com mais de 24 horas permanece armazenado em estado de atencao, sem exclusao silenciosa.

## Ciclo de caixa

- O PDV fecha o caixa chamando `POST /api/v1/cash/sessions/{id}/close`; remover apenas a referência local não encerra uma sessão.
- A migration `0013_single_open_cash_session` cria um índice único parcial para permitir somente uma sessão `open` por tenant/registro.
- Se a migration encontrar duplicatas já abertas, ela aborta e exige reconciliação operacional; não fecha sessões automaticamente.
