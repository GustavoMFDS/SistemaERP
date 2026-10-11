# Leases e inspeção fiscal segura — 11/10/2026

## Limite do incremento

A migração **0042** adiciona token de lease UUID, dono temporário,
vencimento, contador de tentativas, instante de tentativa, diagnóstico
persistido e próximo instante de inspeção em `fiscal_dispatch_jobs`.
Não altera o estado legal de `invoices` nem a numeração fiscal.

`ClaimFiscalPreflight` escolhe somente obrigações **NFC-e 65 de vendas
finalizadas, não históricas, sem invoice existente**, já classificadas
como `awaiting_fiscal_processing`, com referência de certificado
registrada e ambiente do contribuinte em **homologação**. Cada claim
usa `FOR UPDATE SKIP LOCKED`, token gerado no PostgreSQL e vencimento
curto; outra instância não recebe a mesma obrigação durante o lease.

`CompleteFiscalPreflight` exige tenant, venda, token e dono originais
e lease **não vencido** para registrar o resultado. Tentativas
atrasadas após retomada recebem `ErrFiscalLeaseLost`. Falhas temporárias
têm reavaliação programada; diagnósticos incompletos são revisitados
periodicamente sem polling intenso.

`StartFiscalPreflightDiagnostics` roda automaticamente após iniciar
o aplicativo, usando dono aleatório por processo. Executa SOMENTE
consultas locais no PostgreSQL sobre dados mínimos do emitente, NCM,
perfil fiscal por item e formas de pagamento atualmente contempladas.
O painel fiscal exibe o motivo ao operador.

| Código | Interpretação |
| --- | --- |
| `issuer_incomplete` | CNPJ, IE, CRT ou endereço básico incompletos |
| `product_classification_missing` | Sem itens, sem NCM ou perfil fiscal |
| `payment_review_required` | Pagamento exige capacidade fiscal não implementada |
| `inputs_present_unverified` | Campos básicos existem; **NÃO comprova regras tributárias nem emissão** |
| `internal_error` | Erro de inspeção; retentar após backoff |

## Evidência e restrições

- PostgreSQL 16 isolado: **42 migrações + seed** aplicados.
- `TestFiscalDispatchClassifiesWithoutIssuing` validou a exclusividade
  entre instâncias (inclusive tentativas concorrentes reais), retomada de
  lease expirado, rejeição de token/dono antigo e ausência de qualquer
  invoice causada por inspeção. Ao existir invoice ou mudança de estado
  durante a inspeção, a gravação do diagnóstico é recusada.
- `go test ./...`, `go vet ./...`, build do frontend e testes
  Playwright fiscal/impressão **2/2 aprovados**; ESLint com 0 erros
  e 12 avisos já existentes.
- A inspeção de pagamentos exige pelo menos um pagamento registrado
  e reconhece apenas dinheiro, Pix e transferência como opções
  contempladas neste perfil conservador.
- Não há identidade técnica de assinatura/transmissão nem executor SEFAZ
  ligado a essa rotina. O histórico de lease conta diagnósticos, não
  emissões. **Nunca** considerar um resultado de inspeção como
  autorização ou nota fiscal pronta.
- Nenhuma tentativa automática pode avançar para emissão sem um
  executor separado, configuração e credenciais reais, seleção de
  operação/modelo por UF, cálculo e classificação tributária atualizada,
  autorização explícita do responsável, consulta idempotente em caso
  de timeout e homologação oficial.

A implementação segue no PR #25 em **rascunho**, sem merge, sem GitHub
Actions e com transmissão real à SEFAZ desabilitada. As issues #32 e
#33 permanecem abertas.
