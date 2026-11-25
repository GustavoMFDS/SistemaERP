# Registro de decisões e questões em aberto

| ID | Status | Decisão/questão |
|---|---|---|
| DEC-01 | Existente | Manter empresas/CNPJs independentes por tenant; não tratar como filiais automaticamente |
| DEC-02 | Existente | Esterium Local: servidor da loja, terminais LAN, Windows nativo como direção documentada |
| DEC-03 | Proposta | Preservar monólito modular Go/React/PostgreSQL, refatorar incrementalmente |
| DEC-04 | Proposta | Estabilizar antes de expansão funcional; documentação e auditoria podem avançar em paralelo |
| DEC-05 | Proposta | PWA mobile primeiro; acesso remoto privado depois; app nativo só por necessidade |
| DEC-06 | Proposta | IA assistiva read-only por padrão, aprovação humana, nenhuma dependência do PDV |
| DEC-07 | Proposta | Analytics começa com SQL/read models simples e métricas reconciliadas |
| DEC-08 | Proposta | Eventos/jobs persistentes se confiabilidade exigir; avaliar implementação existente |
| DEC-09 | Evidência | LICENSE MIT na main; README divergente. Revisão da marca e licenças de terceiros pendente |
| DEC-10 | Proposta | Comercialização é opcional; foco nas lojas da família |
| DEC-11 | Proposta | Organizar documentos de 2.0 em docs/esterium-2, manter históricos intactos |
| DEC-13 | Aprovada pelo usuário em 24/11/2026 | Setup único nativo Windows: PostgreSQL e Garnet como serviços, ERP compilado e interface web; sem Docker Desktop, WSL, VM Linux, Go, Node ou Python no PC da loja. Docker opcional apenas para desenvolvimento/CI. |
| DEC-12 | Restrição | Sem instalação comercial, emissão fiscal real, merge ou mutação de dados por mera aprovação deste plano |

## Perguntas para descoberta com a família (não bloqueiam auditoria)
- Segmento, número de CNPJs/terminais, volume de SKUs e vendas, processos mais demorados, devoluções e compras.
- Periféricos, bancos/adquirentes, Pix/TEF, crediário, etiquetas, depósitos, validade/lotes e necessidade de e-commerce.
- Método de custo, plano de categorias, comissões, descontos, metas, período e relatórios usados; validar contabilidade.
- Política de backup, tolerância a perda e indisponibilidade (RPO/RTO), PC de teste, responsável de operação e contador.
- Identidade visual, prioridade de mobile/remoto, intenção de disponibilizar a outras empresas, orçamento de integrações e IA.

Registrar respostas em documentação sem dados pessoais ou operacionais sensíveis; decisões específicas passam por aceite. Não presumir que todos os recursos propostos se aplicam à loja.
