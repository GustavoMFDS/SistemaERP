# Etapa 8 — Roadmap (fases)

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
- Fundação NFC-e modelo 65 por tenant: emitente, NCM/CEST, série, referências de CSC/certificado, readiness e metadados de autorização
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
