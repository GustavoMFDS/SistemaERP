# Etapa 8 — Roadmap (fases)

> **Nota de manutenção (2026-10-09):** este roadmap é o plano
> histórico inicial, não um inventário da implementação atual.
> O catálogo de referência, com checklist detalhada de código,
> pendências, validação e prioridades, fica em
> [Catálogo funcional e checklist](catalogo-funcional-sistema.md).
> Por exemplo, fornecedores/compras, gestão de funcionários,
> agenda básica de clientes e a fundação técnica de NFC-e já
> possuem código no PR #15, porém ainda faltam testes efetivamente
> executados, integração operacional e homologação fiscal.
> Não considerar o roadmap uma declaração de produção liberada.


## MVP (entregue aqui)
- Login + RBAC
- Produtos
- Estoque (saldo + movimentações + alerta mínimo + ajuste)
- PDV (criar/finalizar/cancelar venda, pagamentos)
- Financeiro básico (lançamentos + dashboard)
- Preview fiscal de desenvolvimento + armazenamento/download de XML histórico

## v2 — operação não fiscal e fundação NFC-e
- Clientes completo + crediário (contas a receber)
- Compras/fornecedores + contas a pagar
- Impressão de cupom não fiscal (ESC/POS) e integração com gaveta
- Fundação NFC-e modelo 65 por tenant: emitente, NCM/CEST, série, referência do certificado A1, readiness e metadados de autorização; CSC apenas legado opcional para QR v2
- Backup/restore, observabilidade, resiliência/carga e least-privilege validados no ambiente piloto
- Melhorias de performance (cache de permissões, busca full-text refinada)

## v3 — homologação e emissão fiscal real
- Provider SEFAZ NFC-e modelo 65
- Assinatura XML com certificado A1 via secret manager
- Schemas/NTs oficiais vigentes, incluindo alterações de 2026 e Reforma Tributária
- Chave de acesso, QR Code vigente, autorização/retorno e protocolo
- Contingência, cancelamento, inutilização e DANFE-NFC-e
- Homologação por UF/tenant antes de qualquer habilitação em produção
- Integrações Pix/TEF
- E-commerce/omnichannel
- Auditoria avançada e trilhas por entidade
