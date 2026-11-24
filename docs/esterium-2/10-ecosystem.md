# Fase 6 — Esterium Ecosystem (P3)

Expansão opcional após núcleo estável. Confirmar demanda da família e intenção de distribuir comercialmente.

| ID | Capacidade | Contrato e aceite |
|---|---|---|
| ECO-01 | Catálogo digital | Produtos publicados explicitamente, preço/disponibilidade atualizados, não expor custo ou estoque interno indevido; consulta por CNPJ |
| ECO-02 | Orçamentos e pedidos WhatsApp | Orçamento com validade, itens e preço; pedido entra em estado pendente e revisão humana; comunicação consentida, sem mensagens em massa indevidas |
| ECO-03 | Loja virtual/marketplaces | Adaptadores, SKU mapeado, reserva com expiração, confirmação/baixa idempotente, sincronização e reconciliação; não prometer estoque offline |
| ECO-04 | API de integração | Versionamento, credenciais por tenant/escopo, rate limit, webhook assinado/replay protegido, sandbox e documentação; revogação e auditoria |
| ECO-05 | Acesso remoto | Conexão privada autenticada, revogável, autorização por perfil, sem abrir portas públicas do servidor; auditoria e recuperação de acesso |
| ECO-06 | Distribuição comercial | Assinatura/proveniência, atualizações, suporte, onboarding, contrato, proteção de dados e revisão da marca, licenças de terceiros; modelo de negócio a decidir |

## Limites e riscos
Não presumir venda a terceiros, SaaS, compartilhamento entre CNPJs nem dependência obrigatória de nuvem. Licença MIT consta no repositório, não assegura direitos de marca nem licenças das dependências. Revisar custo por integração, contrato de provedor, proteção de dados e portabilidade. Expansão remota não compromete operação local. Documentar desligamento do canal e reconciliação após falha.
