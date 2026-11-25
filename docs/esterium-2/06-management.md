# Fase 3 — Esterium Management (P1)

## Financeiro
**MGT-01 — DRE gerencial:** receita bruta, devoluções/descontos, receita líquida gerencial, CMV, margem bruta, despesas operacionais e resultado gerencial. Definir competência versus caixa, impostos, custos históricos e devoluções com contador; não declarar DRE contábil/fiscal. Registrar custo no evento de venda quando adequado, não recalcular histórico pelo preço de compra atual. Aceite: reconciliação com vendas/ledger e fixture com devoluções.
**MGT-02 — Fluxo de caixa:** realizado e projetado em visões distintas, contas recorrentes, vencimentos, previsão de recebíveis/cartões, cenários e saldos iniciais. Projeção nunca aparece como dinheiro disponível. Aceite: transferência não duplica receita, parcela em atraso destacada, filtros e períodos corretos.
**MGT-03 — Conciliação bancária:** importação OFX/CSV com deduplicação e prévia, correspondência sugerida por referência/valor/data, conciliação parcial/múltipla e divergências auditadas. Nenhum débito/crédito criado sem regra e confirmação. Integração bancária futura depende de provedor, consentimento e segurança.
**MGT-04 — Contas e governança:** categorias/centros de custo, despesas recorrentes, anexos protegidos, aprovações conforme valor, alertas de vencimento, trilha de ajustes e painel de auditoria/LGPD consumindo APIs existentes. Exportar com autorização e retenção adequada.

## Clientes
**MGT-05 — CRM:** vínculo opcional cliente-venda, histórico autorizado, deduplicação assistida, consentimento e revogação, segmentação e preferências de contato. Evitar CPF sem necessidade. Não misturar CNPJs. Aceite: remoção/anonimização quando permitido sem quebrar obrigação legal de retenção.
**MGT-06 — Crediário:** somente se negócio usa venda a prazo própria. Limites, parcelas, vencimento, pagamento parcial, inadimplência, estorno e extrato; política de juros e crédito revisada por responsáveis. Sem concessão automática por IA. Aceite: saldo por parcela, reconciliação e acesso restrito; decisões de crédito transparentes.
**MGT-07 — Fidelidade:** pontos/cashback com regras de elegibilidade, expiração e devolução, ledger de benefícios e prevenção de abuso; sem confundir crédito promocional com depósito bancário.

## Mobile
**MGT-08 — PWA proprietário:** resumo de vendas, caixas, pendências, margem conforme permissão, contas e relatórios, indicação de atualização. Acesso LAN primeiro, remoto em etapa própria.
**MGT-09 — PWA estoque:** câmera/leitor, consulta de SKU/preço, contagem e recebimento, design para uso com uma mão. Não confiar em cache para saldo final; conflitos explícitos. Testar dispositivos reais, permissões da câmera, conexão lenta e offline.

Aceite transversal: totais reconciliados, segregação por CNPJ, acesso mínimo, feedback de erro, exportações seguras, nenhuma mutação silenciosa. Decidir com a família quais rotinas financeiras, crediário e fidelidade realmente são usadas.
