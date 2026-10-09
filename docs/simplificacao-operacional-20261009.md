# Simplificação operacional do SistemaEmGo — 2026-10-09

Branch `ops/pilot-readiness-20260923`, PR #15. Mudanças desenvolvidas e testadas no clone isolado do disco D: da máquina Gustavo.

## Experiência de balcão

- **PDV:** removidos os atalhos textuais do cabeçalho, os quatro indicadores repetitivos em estado normal, a barra 1/2/3 e o menu `Outras ações`. Pendências de sincronização/offline **ainda geram aviso visível** quando existirem.
- **Caixa:** não exibir UUID técnico da sessão. Entrada e retirada de dinheiro (suprimento/sangria) continuam funcionando, mas são ações ocasionais e ficam recolhidas.
- **Venda:** produtos em área ampla, com busca e cartões de escolha. Pagamento ganhou largura, total mais destacado, seleção de forma de pagamento em largura integral e botão de finalização mais evidente.
- **Fechamento:** somente aparece com caixa aberto, preserva valores declarados por forma de pagamento, bloqueia com fila offline pendente e exige conferência humana.
- **Não implementado:** fechamento bancário automático de Pix/cartões; sem resposta real do PSP/adquirente, os pagamentos são lançamentos informados, não dinheiro confirmado.

## Cadastros e outras telas

- **Produtos:** pesquisa automática (sem botão redundante), campo `Código do produto (SKU)`, `Classificação fiscal (NCM)`, `Código tributário (CEST)`; NCM/CEST ficam sob `Dados fiscais` no cadastro. Planilha tem instruções simplificadas, preservando validação e replays seguros.
- **Estoque:** ajuste manual agora pesquisa no servidor por nome, código do produto ou código de barras após digitar duas letras, além da lista inicial, evitando rolar centenas de produtos. A carga inicial foi explicada em termos menos técnicos.
- **Fiscal:** resumo de pendências primeiro; cadastro do emitente, certificado, ambiente e XML histórico em seções recolhidas por padrão. Os hashes SHA-256 foram removidos da tabela visual (continuam na API). Nenhum comando de transmissão foi habilitado.
- **Configurar loja:** progresso expresso como próximos passos, reduzindo indicadores técnicos visíveis; nomes de endereço mais familiares. CNPJ e documentação fiscal continuam fiéis ao cadastro oficial.
- **Funcionários:** lista da equipe é a prioridade; criação e links de acesso individuais ficam em seções opcionais. Mesmo em um único PC, é necessário saber quem realizou cada ação e manter senha própria. Um eventual login local simplificado (nome/PIN ou seleção de operador com desbloqueio seguro) requer especificação, auditoria e novos testes: **não substituído por login compartilhado sem autenticação**.
- **Devoluções:** escolha de venda recente pelo valor e código abre o detalhe sem exigir copiar um UUID. Busca por identificador completo permanece como alternativa, e a liquidação do reembolso continua na etapa financeira.

## Correção financeira em andamento concluída nesta branch

- Listagem de contas com filtro no banco e paginação explícita `limit/offset`, contagem correta e resumo total de contas em aberto por CNPJ calculado fora da página. O comportamento anterior (soma das contas truncadas) podia subestimar saldos.
- Navegação de páginas no painel; fallback quando a API ainda antiga não retorna `summary`: mostra totais indisponíveis em vez de quebrar ou informar zeros falsos.
- Gráfico separa estornos negativos das entradas positivas e identifica saídas e estornos, sem se passar por extrato bancário.

## Testes e limitações

- `go test ./...` e `go vet ./...`: aprovados nas verificações locais anteriores da API financeira.
- `npm run build` e `npm run lint`: aprovados após as alterações, lint com 12 avisos ainda pendentes.
- Playwright no frontend com backend atualizado dedicado: **2/2** testes de paginação e saldos, com resposta real do servidor, aprovados.
- Playwright no frontend conectado a backend de demonstração antigo: **10/11** testes selecionados aprovados; a única falha foi o caso real de paginação ao consultar a API antiga (não envia `limit`/ `summary`). Nessa configuração, o fallback evita quebrar, mas é necessário reiniciar a API atualizada.
- Testes específicos: Fiscal + PDV desktop/celular **3/3**; busca em estoque fora da primeira página **1/1**; seleção de venda recente na devolução **1/1**.
- **Ainda pendente:** validar o endpoint inteiro de devoluções/trocas e reembolso com banco novo isolado; replay concorrente de contas e cancelamentos; suíte E2E completa com fixtures independentes; testes manuais com operador real.
- Ainda não existem conciliação Pix/TEF real, repasse bancário ou homologação NFC-e. Não declarar prontidão de produção.

## Próximas prioridades

1. Subir a API nova e isolar ambiente por conjunto E2E, zerando sujeira de caixa anterior sem apagar trilhas auditáveis de produção.
2. Realizar teste funcional completo de devolução, reembolso e movimento de estoque em banco isolado.
3. Desenhar login de operadores de balcão em um PC com autenticação rápida, sem perder responsabilização.
4. Implementar integração de pagamentos verificados e, só então, viabilizar preenchimento/fechamento automático com dados confiáveis.
5. Revisar a experiência do Configurar loja e Fiscal com alguém que nunca usou um PDV, após fechar as dependências de segurança.

Não realizar merge nem liberar o sistema para operação fiscal/financeira real com os gates pendentes.
