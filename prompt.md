Quero que você atue como um arquiteto de software sênior, product designer e desenvolvedor full-stack especialista em sistemas ERP/PDV para o mercado brasileiro.

Sua tarefa é projetar e construir um sistema completo de PDV + ERP moderno, com backend robusto, frontend profissional, banco de dados relacional, autenticação, regras de negócio, documentação técnica e base preparada para evolução futura.

Objetivo do sistema:
Criar um sistema de gestão comercial para pequenas e médias empresas, com foco em operação de loja, controle de estoque, financeiro, vendas e preparação para rotina fiscal com NF-e.

Stack desejada:
- Backend em Go (Golang).
- API REST bem estruturada.
- PostgreSQL como banco principal.
- Frontend web responsivo para ERP administrativo.
- Frente de caixa/PDV com interface rápida, simples e otimizada para operação.
- Docker para ambiente local.
- JWT ou sessão segura para autenticação.
- Arquitetura modular, limpa e escalável.
- Logs, tratamento de erros, validações e organização pronta para produção.

Quero que você entregue:
1. Arquitetura completa do sistema.
2. Estrutura de pastas do backend e frontend.
3. Modelagem do banco de dados.
4. SQL de criação das tabelas principais.
5. Endpoints da API.
6. Regras de negócio detalhadas.
7. Telas do sistema e fluxo entre elas.
8. Código inicial funcional do projeto.
9. Seed de dados para testes.
10. Documentação para rodar localmente.
11. Plano de evolução futura.
12. Boas práticas de segurança e auditoria.

Módulos obrigatórios do sistema:

1) Controle de estoque
O sistema deve possuir um módulo de estoque completo com:
- Cadastro de produtos.
- SKU/código interno.
- Código de barras opcional.
- Nome do produto.
- Descrição.
- Categoria.
- Preço de custo.
- Preço de venda à vista.
- Preço promocional opcional.
- Quantidade atual em estoque.
- Estoque mínimo.
- Unidade de medida.
- Status ativo/inativo.
- Histórico de movimentações.

Regras do estoque:
- Quando alguém comprar um produto no PDV, a quantidade vendida deve ser automaticamente subtraída do estoque.
- O sistema deve impedir venda acima da quantidade disponível, exceto se houver configuração permitindo venda sem estoque.
- Deve existir alerta visual e relatório para produtos com estoque baixo quando a quantidade atual for menor ou igual ao estoque mínimo.
- Toda entrada e saída deve gerar movimentação de estoque com data, usuário, tipo e observação.
- Deve existir tela de ajuste manual de estoque com motivo obrigatório.
- Deve existir entrada de estoque por compra, devolução ou ajuste.
- Deve existir saída de estoque por venda, perda, avaria ou ajuste.
- Deve existir busca rápida por nome, SKU e código de barras.

2) Controle financeiro
O sistema deve possuir módulo financeiro com:
- Contas a pagar.
- Contas a receber.
- Registro de despesas.
- Registro de receitas.
- Controle de vendas.
- Controle de lucro.
- Fluxo de caixa.
- Fechamento de caixa diário.
- Formas de pagamento.
- Relatórios por período.

Regras do financeiro:
- Toda venda finalizada deve gerar lançamento financeiro.
- O sistema deve separar valor bruto, desconto, valor líquido e lucro estimado.
- O lucro deve ser calculado com base no preço de venda menos custo do produto.
- Deve haver dashboard com total vendido no dia, no mês, ticket médio, lucro, despesas e saldo em caixa.
- Deve ser possível filtrar por período, operador, forma de pagamento e status.
- Deve existir controle de sangria e suprimento de caixa.
- Deve existir fechamento de caixa com conferência de valores por forma de pagamento.
- Deve existir histórico financeiro auditável.

3) Menu de vendas / PDV completo
Quero um módulo de vendas com interface rápida, ideal para balcão/caixa, contendo:
- Campo de pesquisa de produtos no estoque.
- Busca por nome, código e código de barras.
- Lista de produtos com nome e preço à vista.
- Botão para adicionar produto ao carrinho.
- Carrinho com itens, quantidade, desconto e subtotal.
- Alteração manual de quantidade.
- Remoção de item.
- Aplicação de desconto percentual ou valor fixo conforme permissão do usuário.
- Identificação opcional do cliente.
- Seleção de forma de pagamento.
- Finalização da venda.
- Impressão ou geração de comprovante.
- Histórico de vendas.

Regras do PDV:
- A tela deve ser rápida, com foco operacional.
- Ao adicionar item ao carrinho, recalcular subtotal, total e impacto no estoque.
- Ao finalizar a venda, gerar registro da venda, itens da venda, movimentação de estoque e lançamento financeiro.
- Deve existir status da venda: aberta, finalizada, cancelada.
- Cancelamento deve exigir permissão e justificativa.
- Se a venda for cancelada após conclusão, o estoque deve ser estornado e o financeiro revertido conforme regra definida.
- Deve existir suporte para múltiplas formas de pagamento na mesma venda, se possível.
- Deve existir abertura e fechamento de caixa por operador.

4) Nota fiscal NF-e
Quero um módulo fiscal inicial com página para gerar XML de NF-e.
Esse módulo pode começar como MVP documental, sem integração completa com SEFAZ neste primeiro momento, mas deve ser estruturado para futura evolução.

O módulo NF-e deve conter:
- Tela para selecionar uma venda já finalizada.
- Dados do emitente.
- Dados do cliente/destinatário.
- Dados dos produtos.
- Quantidades.
- Valores unitários.
- Valores totais.
- CFOP, NCM, CST/CSOSN e demais campos fiscais essenciais configuráveis.
- Geração do XML da NF-e.
- Armazenamento do XML gerado.
- Histórico de XMLs gerados.
- Possibilidade de download do XML.

Regras da NF-e:
- O sistema deve gerar um XML estruturado e validável internamente.
- Deve haver camada separada para montagem do XML.
- O XML deve ficar salvo no sistema, pois o armazenamento correto desses arquivos é exigência legal e operacional.
- A arquitetura deve ficar preparada para futura assinatura digital, transmissão, retorno de protocolo e DANFE.
- Cada XML deve estar vinculado à venda que o originou [web:67][web:61].

Requisitos de interface:
- Interface moderna, limpa e profissional.
- Sidebar com módulos: Dashboard, Produtos, Estoque, Vendas, PDV, Financeiro, Clientes, Fiscal/NF-e, Relatórios, Configurações.
- Layout responsivo.
- Tema claro/escuro se possível.
- Tabelas com busca, filtros e paginação.
- Formulários com validação.
- Alerts para estoque baixo e erros operacionais.
- Dashboard com cards e gráficos principais.

Perfis de usuário:
- Admin.
- Gerente.
- Operador de caixa.

Permissões:
- Admin pode tudo.
- Gerente pode operar vendas, estoque, relatórios e financeiro.
- Operador de caixa pode abrir caixa, vender, consultar produtos e finalizar vendas.
- Cancelamento de venda, ajuste de estoque e geração fiscal sensível devem depender de permissão.

Entidades principais do banco:
- users
- roles
- permissions
- customers
- products
- categories
- inventory_movements
- cash_registers
- cash_sessions
- sales
- sale_items
- payments
- accounts_payable
- accounts_receivable
- expenses
- revenues
- invoices
- invoice_xml_files
- companies
- audit_logs

Regras técnicas obrigatórias:
- Use arquitetura em camadas: handler/controller, service, repository, domain/model.
- Separe regras de negócio do acesso ao banco.
- Use migrations.
- Crie seeds para usuários, produtos e categorias.
- Implemente validação de payload.
- Implemente middleware de autenticação.
- Implemente middleware de autorização por perfil/permissão.
- Adicione logs estruturados.
- Adicione auditoria de ações críticas.
- Crie documentação de endpoints.
- Crie arquivo docker-compose para subir aplicação e PostgreSQL.
- Crie README com instruções detalhadas.

Fluxos obrigatórios:
Fluxo de venda:
1. Operador abre caixa.
2. Pesquisa produto.
3. Adiciona item ao carrinho.
4. Sistema consulta estoque disponível.
5. Finaliza venda.
6. Sistema grava venda e itens.
7. Sistema dá baixa automática no estoque.
8. Sistema gera lançamento financeiro.
9. Sistema disponibiliza dados para geração futura de NF-e.

Fluxo de estoque:
1. Usuário cadastra produto.
2. Usuário define estoque mínimo.
3. Usuário realiza entrada de mercadoria.
4. Sistema atualiza saldo.
5. Sistema alerta quando atingir nível mínimo.

Fluxo fiscal:
1. Usuário acessa módulo NF-e.
2. Seleciona venda finalizada.
3. Sistema carrega emitente, cliente e itens.
4. Usuário complementa campos fiscais obrigatórios.
5. Sistema gera XML.
6. XML fica salvo e disponível para download.

Quero também:
- sugestões de índices no banco;
- regras de consistência transacional;
- uso de transações para venda + estoque + financeiro;
- exemplos de payload JSON;
- exemplos de respostas da API;
- tratamento de erros comuns;
- estratégia para evitar inconsistência de estoque;
- roadmap em fases: MVP, v2 e v3.

Prioridade do MVP:
- Login.
- Cadastro de produtos.
- Controle de estoque.
- Alertas de estoque mínimo.
- PDV com carrinho e finalização.
- Baixa automática do estoque.
- Controle financeiro básico.
- Geração e download de XML de NF-e em modo MVP.

Quero que você entregue o resultado em etapas organizadas:
Etapa 1: visão geral e arquitetura.
Etapa 2: modelagem de dados.
Etapa 3: backend em Go.
Etapa 4: frontend.
Etapa 5: fluxo de negócio.
Etapa 6: módulo fiscal XML.
Etapa 7: segurança, auditoria e deploy.
Etapa 8: roadmap.

Importante:
- Pense como sistema real de mercado brasileiro.
- Não faça apenas exemplo genérico.
- Estruture o sistema para crescimento.
- Escreva código limpo e com responsabilidade de produção.
- Explique as decisões arquiteturais.
- Priorize confiabilidade de estoque, rastreabilidade financeira e organização fiscal.