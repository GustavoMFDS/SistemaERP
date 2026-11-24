# Fase 5 — Esterium Intelligence (P2)

Objetivo: recomendações explicáveis que ajudem o gestor a reduzir ruptura, perdas e capital parado. A IA é opcional, desacoplada do caminho crítico de venda. Sem ações financeiras, fiscais, de preço, estoque ou crédito autônomas.

## INT-01 — Preparação de dados
Usar eventos e métricas reconciliados, séries por SKU/tenant, estoque disponível, períodos de ruptura, promoções e lead time. Dados faltantes/ruptura censuram demanda observada; não assumir zero demanda. Separar treino/validação temporal para evitar vazamento. Dados sintéticos para desenvolvimento; política de retenção e consentimento antes de uso real.

## INT-02 — Previsão de demanda
Começar com naive/seasonal naive, média móvel, suavização exponencial; comparar alternativas somente se houver histórico suficiente. Backtesting rolling-origin, MAE/WAPE (denominador não zero), viés e cobertura de intervalos; avaliar por volume e SKU. Para itens intermitentes, avaliar método adequado ou fallback por regra. Mostrar horizonte, versão, intervalo e última atualização. Sem prometer precisão. Exemplo fictício: previsão 58/63/67/72 unidades em quatro semanas, total 260, estoque 85; compra depende de lead time, pedidos em trânsito e segurança.

## INT-03 — Reposição assistida
Demanda durante lead time + segurança - saldo disponível - entradas confirmadas, respeitando unidade de compra, mínimo e orçamento. Mostrar cálculo, premissas e alternativas; gestor aprova pedido normal via regras de compras. Aceite: sem quantidade negativa, sem dupla compra e fallback quando modelo indisponível.

## INT-04 — Anomalias
Divergências de caixa, devoluções, descontos, perdas, custos e quedas de venda. Começar por regras e estatística robusta por contexto; avaliar falsos positivos, precisão em casos revisados e alert fatigue. Exibir evento, baseline e motivo. Alertas são hipóteses, não acusações; nenhum bloqueio punitivo automático.
 
## INT-05 — Precificação assistida
Simular margem e cenários de preço, respeitar custos, tributos, promoções e limites definidos. Elasticidade só com evidência suficiente; sem inferir causalidade de correlação. Recomendação com explicação e aprovação, preço final sempre pelo domínio de catálogo.

## INT-06 — Assistente empresarial
Perguntas em linguagem natural: "Quais produtos devo comprar?", "Por que minha margem caiu?", "Quanto tenho em estoque parado?", "Qual a previsão de dezembro?". Usar ferramentas de consulta read-only com parâmetros tipados, RBAC/tenant e limites; nunca acesso SQL irrestrito ou execução de instruções oriundas de dados. Responder com período, métricas, fonte e limitações; admitir desconhecimento. Proteger contra prompt injection, exfiltração, logs sensíveis e vazamento entre empresas. Se provedor externo, minimizar dados, revisar política de processamento, custo e consentimento; oferecer desativação. Não enviar dados reais por padrão.

## Arquitetura e aceite
Core -> read models -> jobs de previsão/anomalia -> recomendações -> API autorizada. Avaliar Go ou Python separado apenas se biblioteca/modelo justificar; interface versionada, modelo registrado, reprodução e rollback. Testar isolamento, acesso proibido, dados escassos, indisponibilidade, desvio do modelo e regressão. Nenhuma dependência de LLM para vender. Só promover modelo que supere baseline sob métrica e janela definidas; aprovação humana para uso.
