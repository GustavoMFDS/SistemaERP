# NFC-e: recuperação consultiva segura (MG) — 12/10/2026

## Contrato e estado atual

A autorização real de NFC-e só existe após resposta genuína da SEF/MG, assinatura válida, protocolo e arquivamento transacional do XML processado `nfeProc`. Uma venda finalizada não se torna autorizada por ter uma obrigação fiscal, um XML local, uma sequência reservada, um PDF/DANFE ou uma consulta pendente. A impressão é uma decisão posterior e independente da emissão fiscal.

O serviço fiscal já possui um caminho `AuthorizeNFCeHomologation` que, quando encontra uma NFC-e **signed**, grava **submitted** no banco antes de invocar o gateway; se encontrar **submitted**, faz **Consult** por chave, nunca **Authorize** novamente às cegas. Um timeout após o POST inicial não permite saber com segurança se a SEF/MG recebeu a nota. A consulta poderá inclusive retornar temporariamente `217` (não consta); isso **não significa** que seja seguro repetir o envio.

### Ajuste de interpretação dos retornos

- `100` + mesma chave de acesso + protocolo genuíno + data/hora válida é o único caminho aceito pelo parser de consulta para o status local `authorized`. A rotina existente de persistência ainda exige XML assinado e vínculo de protocolo no `nfeProc`.
- `101` (cancelada), `110` (denegada), `217` (não encontrada), duplicidade `204`/`539` e demais códigos **não são convertidos em `rejected`** por uma consulta posterior. Permanecem em `submitted`, exigindo recuperação/avaliação. Uma consulta administrativa pode precisar identificar cancelamento ou denegação anterior: o código não deve disparar uma nova NFC-e.
- Resposta de envio com duplicidade `204`/`539` tampouco vira rejeição definitiva. Impõe consulta da chave e, se persistir a incerteza, revisão humana.
- Não inferir autorização ou registro de cancelamento a partir de mensagens textuais, códigos isolados, ou ausência da chave em uma única consulta.

## Fila durável, segura e por CNPJ (migração 0046)

`nfce_consult_recovery_jobs` contém, por par `tenant_id, invoice_id`, apenas identificação técnica da NFC-e, chave de acesso, ambiente de **homologação**, estado, número de tentativas, momento da próxima consulta, lease e último diagnóstico. Não armazena senha, PFX, token CSC ou qualquer material de assinatura.

- O planejamento periódico, executado pelo próprio servidor com `StartNFCeConsultationPlanner`, **somente** insere documentos modelo 65 já `submitted` em MG/homologação, e marca como `resolved` itens cujo status já foi efetivamente finalizado por outro fluxo. Não chama a SEF/MG, não cria número, não transmite, não assina e não modifica notas.
- O planejamento é idempotente pelo par empresa/nota; a transição para `resolved` observa exclusivamente o status da tabela de notas.
- `ClaimNFCeSubmittedConsultations` reserva trabalhos vencidos usando identidade técnica ativa, `FOR UPDATE SKIP LOCKED`, token exclusivo e expiração. Exclui notas que deixaram `submitted` ou não estão em homologação, e é limitado a 10 tentativas.
- `CompleteNFCeConsultation` usa `tenant_id`, nota, worker e token válido antes de soltar o lease. Registra apenas diagnóstico (`pending`, `not_found`, `transport_error`, `invalid_protocol`, `manual_review`), com espaçamento de 5–60 minutos. Não aceita `authorized` ou `rejected` como resultado; tampouco executa uma nova autorização fiscal.
- Após 10 tentativas ou escalonamento explícito, a tarefa fica em `manual_review`, **sem novos claims automáticos**. Diagnósticos anteriores não desaparecem se o operador recarregar a página. A migração de rollback é fail-closed diante de registros existentes.

**Limite de execução:** os métodos de claim/complete são a preparação para um executor futuro de **consulta exclusiva por chave**, com integração ao serviço de arquivamento real. Esse executor remoto **não está conectado ao loop do servidor** nesta rodada; o loop atual apenas prepara a fila. Não habilitar a transmissão nem considerar a recuperação completa sem certificado real, homologação SEF/MG, idempotência transacional E2E, processamento de diferentes cStat e ensaio de perda de conexão após envio.

## Validações locais

- `go test ./...` e `go vet ./...`.
- Testes unitários do adaptador SEFAZ para códigos `100`, `101`, `110`, `204`, `217`, `539`, `656`, incluindo respostas com e sem protocolo.
- Migrações **0001–0046** aplicadas em PostgreSQL descartável.
- `TestNFCeConsultationRecoveryIsTenantScopedAndFenced` usa duas lojas fictícias e dois trabalhadores independentes. Comprova planejamento idempotente, ausência de colisão de leases, rejeição de trabalhador errado e repetição de token, isolamento de CNPJs, diagnóstico sem mutação da nota e resolução por status final já gravado.
- Somente o banco de testes descartável foi alterado; nenhuma venda ou certificado real.

## Resultados comprovados da rodada (12/10/2026)

- `go test ./...` e `go vet ./...` passaram com o adaptador de consulta alterado; a nova regra é conservadora inclusive para retornos de cancelamento/denegação em respostas de envio.
- Instância PostgreSQL 16 descartável em porta isolada: **46 migrações up** aplicadas com dados sintéticos. A migração `0046.down` recusou apagar jobs existentes; em outra base vazia `0046.down` e reaplicação `0046.up` passaram.
- `TestNFCeConsultationRecoveryIsTenantScopedAndFenced` executado com as flags explícitas de banco descartável, corrigido para não pressupor ordem de lease e para gerar CNPJ/numeração próprios a cada execução. **Dois runs consecutivos na mesma base** passaram.
- Não foram executados calls SOAP, autorização real, reenvio de XML, homologação SEF/MG ou testes de certificado. A migração e o planejador podem ser instalados sem habilitar transmissão, mas o executor remoto ainda é P0 pendente.
