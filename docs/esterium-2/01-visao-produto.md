# Visão do produto e escopo

## Problema
A loja precisa vender rapidamente, controlar saldos e dinheiro, comprar no momento certo e enxergar resultados sem depender de planilhas desconectadas. O sistema deve funcionar durante falhas previsíveis, sem esconder erros ou confundir registro operacional com confirmação bancária/fiscal.

## Perfis
| Perfil | Objetivo | Experiência |
|---|---|---|
| Proprietário | Entender situação, margem, caixa, riscos | Início, analytics e alertas; visão mobile |
| Gerente | Coordenar operação e aprovar exceções | Pendências, compras, permissões, relatórios |
| Vendedor | Localizar produto, preparar venda, atender | Busca rápida, disponibilidade, orçamentos |
| Caixa | Cobrar, finalizar, fechar e devolver | Scanner, teclado, mensagens claras e fluxo rápido |
| Estoque | Conferir e contar mercadorias | Leitura de código, contagem, divergências e recebimento |
| Contador/administrativo | Conferir dados e documentos | Exportações autorizadas, fiscal, financeiro e trilhas |
| Técnico | Instalar e recuperar sem perda | Diagnóstico, logs saneados, backup, runbook |

Perfis novos são propostas; mapear cada um ao RBAC existente antes de implementar.

## Princípios
- Operação local em servidor Windows com terminais LAN e PWA, sem exigir internet para os fluxos locais suportados.
- CNPJs independentes, ainda que da mesma família. Sem compartilhamento implícito de catálogo, clientes, contas, estoque ou fiscal.
- Verdade financeira e fiscal no backend; não considerar pagamentos confirmados por mero registro ou notas autorizadas por prévia.
- UX de balcão primeiro; operações irreversíveis confirmadas e auditadas.
- IA recomenda e explica; não toma decisões financeiras, comerciais ou fiscais autonomamente.
- Go, React e PostgreSQL mantidos; reduzir complexidade desnecessária.
- Comercialização, multi-depósito, crediário e e-commerce dependem de adequação ao segmento e validação com a família.

## Métricas a medir antes de definir metas
Tempo de venda p50/p95 (scanner até confirmação), erro e repetição de venda, divergência de caixa, acurácia de estoque contado, ruptura, capital parado, tempo para receber compra, tempo de conciliação, disponibilidade no horário comercial, sucesso de backup e tempo de recuperação. Separar estimativas de números observados. Metas quantitativas propostas somente após baseline em ambiente representativo.

## Limites
Não substituir contador, adquirente ou autorização SEFAZ; não construir contabilidade completa nem banco digital. Módulos novos não liberam operação real. Usar dados fictícios em testes, nunca dados identificáveis no GitHub público.
