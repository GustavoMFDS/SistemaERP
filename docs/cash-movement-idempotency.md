# Idempotência de sangria e suprimento

Movimentos manuais de caixa usam `POST /api/v1/cash/sessions/{id}/movements` e exigem o header `Idempotency-Key`.

O backend normaliza o payload, calcula um hash incluindo sessão, tipo, valor e observação e serializa concorrência por tenant/operação/chave usando advisory lock. Movimento de caixa, lançamento no ledger, resultado idempotente e auditoria são persistidos na mesma transação.

Comportamento:

- chave nova + payload válido: `201`, `replayed=false`;
- mesma chave + mesmo payload: retorna o mesmo movimento com `200`, `replayed=true`;
- mesma chave + payload diferente: `409 conflict`;
- chave ausente: `422 validation_error`.

No PDV, uma tentativa ambígua mantém a mesma chave enquanto sessão, tipo e valor não mudarem. Isso torna seguro repetir a operação após perda de resposta sem criar uma segunda sangria/suprimento.

A implementação reutiliza `finance_idempotency_keys`; não cria uma segunda infraestrutura de idempotência nem exige migration adicional.
