# Offline PDV (Etapa 13)

Objetivo: permitir que o PDV continue operando em quedas de rede **curtas/intermitentes**, registrando vendas localmente e sincronizando depois, sem duplicar lançamentos.

## Escopo (MVP)

- Persistência local: fila de requisições no `localStorage` (browser)
- Fila de eventos: cada venda (`POST /api/v1/sales`) é registrada como um item na fila
- Sincronização posterior: ao voltar online, o frontend drena a fila em ordem
- Segurança/consistência: idempotência no backend para retries (`Idempotency-Key`)

## Frontend

- Página: `web/src/pages/PDVPage.tsx`
- Fila: `web/src/lib/offlineQueue.ts`

Fluxo:
1. PDV monta e tenta sincronizar pendências se `navigator.onLine`.
2. Em `Finalizar`, se offline (ou erro de rede), a venda é enfileirada com um `Idempotency-Key`.
3. Evento `online` dispara `flushQueue()` para sincronizar.

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

- Itens pendentes expiram apos 24 horas no frontend.
- A fila guarda somente o payload necessario para recriar a venda e o cabecalho `Idempotency-Key`.
- Evite incluir dados pessoais sensiveis no payload de venda offline. Quando o cliente for opcional, prefira venda sem identificacao.
- `localStorage` nao e um cofre criptografico. Nao ha chave segura no frontend para criptografia forte sem apoio do usuario/dispositivo.
- A funcao `clearOfflineQueue()` permite limpeza manual controlada quando o operador precisar descartar pendencias locais.
- Logout chama a limpeza da fila local para evitar que pendencias de um operador fiquem disponiveis para outro usuario no mesmo navegador.
- Cabecalhos sensiveis como `Authorization`, cookies e tokens nao sao persistidos na fila.
- O backend usa `request_hash`: mesma chave + mesmo hash reaproveita o resultado; mesma chave + hash diferente retorna `409 conflict`.

