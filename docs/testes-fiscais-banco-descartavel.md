# Testes fiscais com banco descartável

A numeração de NFC-e e seu histórico técnico são dados sensíveis à auditoria. Um teste que grava sequências fiscais não pode fazer `DELETE` nas sequências de uma empresa real, nem depender de `force` para remover um registro protegido.

## Execução isolada no CI

O job `integration` cria **uma base de dados separada**, `sistemaemgo_fiscal_disposable_ci`, exclusivamente para executar:

- `TestNFCeFoundation_TenantIsolationAndConstraints`
- `TestNFCeReservation_IsAtomicAndIdempotentPerSale`

A execução migra a base vazia, carrega dados sintéticos, liga `TEST_FISCAL_DISPOSABLE_DB=1` **somente no passo dessa base** e executa a suíte uma vez (`-count=1 -p 1 -parallel 1`). Ao término, remove **a base que o próprio passo criou**, mesmo se um teste falhar. A rotina se recusa a usar uma base preexistente com esse nome.

O teste de fundação requer a flag explícita. A suíte padrão continua usando sua própria base e não habilita automaticamente os testes que criam histórico fiscal imutável.

## Critérios de segurança

1. Não configurar a flag num ambiente de loja, homologação compartilhada ou base de produção; o nome da variável sozinho não torna uma base segura.
2. Não executar `DELETE FROM fiscal_document_sequences` ou excluir configurações fiscais apenas para passar um teste.
3. Não usar `force`, desabilitar gatilhos, adulterar protocolo/autorização nem falsificar um resultado SEFAZ.
4. Não reaproveitar a mesma base para a segunda execução: criar **outra base vazia**, migrar, semear, executar e descartá-la.
5. Não confundir a aprovação desses testes de persistência e isolamento com homologação SEFAZ MG ou liberação de emissão real.

Para validar um segundo ciclo, o pipeline deve criar uma segunda base descartável em execução separada ou repetir a execução inteira num ambiente efêmero novo, nunca zerar manualmente os registros fiscais da primeira.
