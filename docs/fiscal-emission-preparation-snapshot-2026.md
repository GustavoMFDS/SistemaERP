# Preparação fiscal versionada por venda — 11/10/2026

## Motivação

Uma inspeção fiscal genérica não pode ser reutilizada como autorização
para emitir após alterações no carrinho, pagamento, produto, CNPJ,
configuração ou impostos. A migração **0044** cria a tabela
`fiscal_emission_preparations`, com um registro **imutável por lease
concluído** para identificar exatamente os dados examinados.

O fluxo automático hoje segue:

1. Cada venda cria obrigação fiscal e fila por CNPJ (0039/0041).
2. A instância técnica assume um lease com fencing, sem duplicar
   trabalho (0042/0043).
3. O backend lê os dados de venda, situação da obrigação, emitente,
   referência/configuração fiscal, itens, NCM, perfis fiscais, valores
   e pagamentos, numa serialização JSONB determinística. Calcula um
   SHA-256 e **guarda apenas o hash**, não dados pessoais, certificados,
   segredos ou o JSON integral.
4. No momento de concluir, uma transação PostgreSQL verifica:
   lease, dono, identidade não revogada, empresa, venda finalizada,
   NFC-e modelo 65 em homologação e ausência de nota existente.
   **Recalcula o hash e o diagnóstico**. Se a venda mudou ou se
   alguém tentou registrar indevidamente um resultado otimista, recusa
   o plano.
5. Plano, atualização do lease e auditoria da conclusão são gravados
   **na mesma transação**. Queda, token inválido, lease vencido ou
   entrada divergente resultam em rollback, sem marcar emissão.
6. O plano tem apenas dois estados:
   `blocked` e `tax_review_required`. O segundo significa que
   alguns campos básicos existem, **não** que os impostos estão
   corretos ou que a nota está pronta para envio.
7. `GET /api/v1/fiscal/obligations/{saleID}/preparation` exige
   `invoice:read` e filtra por CNPJ autenticado. Ele recalcula o
   SHA-256 dos dados atuais e retorna `source_matches` para mostrar
   alteração posterior. Não entrega hash, segredo nem payload de venda.
   O painel Fiscal apresenta o estado junto ao histórico técnico.

## Cobertura dos dados usados no hash

A consulta inclui: status e valores da venda, vínculo do cliente,
tipo de obrigação e `legacy_review`, existência de invoice,
identificação/endereço do emitente, ambiente, série, presença da
referência de certificado/CSC e revisão de configuração, itens
ordenados por ID com NCM, CEST, preço, quantidade, desconto,
perfil fiscal e suas versões, e pagamentos ordenados por ID
com modalidade e valor.

**Não é prova de conformidade fiscal.** Um documento legalmente
válido exige validações reais de NCM/CFOP/CST, códigos IBS/CBS,
cliente, forma de pagamento, unidade federativa e evento.
Futuro executor deverá **recalcular o hash imediatamente antes de
reservar numeração ou transmitir**, sem confiar num plano antigo.
A presente implementação não contém o executor e não faz esse salto.

## Testes locais

- PostgreSQL 16 em instância temporária: **44 migrações + seed**.
- Teste integrado `TestFiscalDispatchClassifiesWithoutIssuing`:
  lease exclusivo/recuperável, identidade técnica e auditoria,
  hash rejeitado quando o endereço muda, rejeição de classificação
  otimista incompatível, ausência de plano depois
  da falha, nova preparação atômica, proibição de replay de lease,
  imutabilidade do plano, isolamento entre empresas, detecção
  posterior de alteração de endereço e de inclusão de pagamento,
  e **zero invoices criadas** pela preparação.
- `go test ./...` e `go vet ./...` aprovados;
  frontend `npm run build` aprovado; Playwright Chromium
  painel Fiscal/PDV **2/2**; lint sem erros (12 avisos conhecidos).
- Testes utilizam dados sintéticos locais. Não houve comunicação
  com a SEFAZ, certificação oficial nem emissão real de nota.

## Próxima etapa

A emissão automática P0 da issue #32 permanece aberta. Ainda
faltam o executor fiscal verdadeiro, suporte ao modelo NF-e 55,
regras tributárias por UF, configuração real de certificado e CSC,
homologação com SEFAZ, identificação do destinatário quando exigida,
suporte completo a pagamentos, reconciliação de timeouts e retenção
do XML autorizado. Não habilitar produção e não marcar o PR pronto.

Não executar GitHub Actions. Não fazer merge enquanto a emissão
oficial não for validada e explicitamente autorizada.
