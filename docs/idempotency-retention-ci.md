# Retenção de idempotência — prova em quatro tabelas

A rotina real `startIdempotencyMaintenance` em app.go remove chaves com mais de 30 dias, em lotes de 2.000, até cinco por tabela por execução. A proteção do navegador #220 limita replay a 28 dias. A rotina não deve eliminar registros comerciais.

O gate CI cria banco exclusivo sistemaemgo_retention_ci, aplica todas as migrations e seed, constrói dois tenants sintéticos e insere 2.001 chaves antigas por tabela para obrigar segundo lote, chaves recentes (29 dias e 1 dia) e chave homônima entre tenants. Executa a rotina real duas vezes (sem esperar ticker de uma hora), exige zero antigas e preservação de ambas recentes, vendas e devoluções. Inclui sales, procurement, returns e finance. Em tabelas com resource_id polimórfico usa identificadores sintéticos, sem alegar validar operações comerciais específicas. Não reutiliza banco existente; cleanup é do banco dedicado. Não executa em ambiente local ou real. O teste histórico #8 cobria apenas vendas e tinha cleanup dependente de pool já fechado; este incremento não altera política de retenção nem amplia autorização.

Limites: teste não prova o ticker de uma hora, comportamento sob crash do PostgreSQL, restore de backup, ou concorrência com múltiplos processos de manutenção. CI precisa ficar verde no SHA exato. Não há alteração de produção.
