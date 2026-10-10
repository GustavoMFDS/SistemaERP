# NFC-e autorizada: cadeia do protocolo SEFAZ e nfeProc (10/10/2026)

Este incremento prepara a distribuição do XML processado, **sem transmitir
documentos nem habilitar emissão em produção**. Não confundir nota assinada
com nota autorizada nem a obrigação fiscal por venda com a autorização.

## Fluxo técnico

1. A resposta de `retEnviNFe` ou `retConsSitNFe` recebida pelo gateway
   SEFAZ é analisada. O elemento original `protNFe` é extraído do
   XML da resposta e mantido em bytes de uso **interno**. Não é reconstruído
   a partir de número ou status digitado pelo operador.
2. O `cStat=100` sozinho não basta: são exigidos chave de acesso,
   protocolo e carimbo de recebimento. Na montagem de `nfeProc`, também
   devem coincidir modelo NFC-e 65, ambiente, chave, digest assinado
   (`Reference/DigestValue` = `protNFe/infProt/digVal`), número de
   protocolo e instante de recebimento. A rotina rejeita divergências.
3. A aplicação monta o arquivo `<chave>-procNFe.xml` com a NFe
   **assinada preservada** e o fragmento `protNFe` capturado.
4. O status `authorized` e o armazenamento de `protocol_xml`,
   `processed_xml` e hash SHA-256 são gravados **na mesma transação**,
   via migration 0040. Se falhar a montagem, arquivo, auditoria ou commit,
   a nota permanece no estado anterior (normalmente `submitted`).
   Na próxima tentativa, deve-se consultar a chave, não reenviar cegamente.
5. Para obter o XML final, `GET /api/v1/fiscal/nfce/invoices/{invoiceID}/processed-xml`
   exige `invoice:read`, filtra pelo CNPJ/tenant autenticado, verifica
   estado `authorized`, modelo 65 e hash do conteúdo, registra auditoria
   e retorna `application/xml` com `Cache-Control: no-store`.
   O download separado de XML técnico não equivale ao arquivo autorizado.
   Após cancelamento, o arquivo segue arquivado, mas esta rota de
   **download como autorizado** o bloqueia.

## Testes e limites conhecidos

- Testes unitários do builder em Go **passaram antes da integração**:
  documento genuinamente assinado com RSA de testes e protocolo de
  **fixture sintética** geraram `nfeProc`; casos com ambiente, digest,
  chave, cStat, protocolo e data trocados foram rejeitados.
- Em 10/10/2026 o ZIP oficial **PL_010f_v1.04** do Portal Nacional
  (SHA-256 `B8589490A58A09A993A80E6AC4D7ED10F20892061ECFC56719337098D4B95998`)
  foi baixado novamente. Ele inclui o tipo `TNfeProc` em
  `leiauteNFe_v4.00.xsd`, mas **não** inclui `procNFe_v4.00.xsd`
  como arquivo separado. O script de validação cria localmente apenas
  a declaração de elemento raiz `nfeProc` referindo-se ao tipo
  **oficial e não modificado** e às dependências oficiais.
  Duas NFC-e assinadas (legada e IBS/CBS) e **dois nfeProc sintéticos**
  gerados pelo Go **passaram** em `lxml.XMLSchema`.
  Isso prova estrutura nesses casos, jamais protocolo SEFAZ real.
- A migração **0040 passou**, junto com **todas as 40 migrações**
  e o seed em PostgreSQL 16 temporário isolado (porta 55439).
  O teste integrado `TestNFCeReservation_IsAtomicAndIdempotentPerSale`
  passou **duas vezes**, cada vez com um banco recém-criado,
  exercitando recuperação por consulta, persistência atômica do
  `nfeProc`, consulta por tenant e bloqueio de download após
  cancelamento. `go test ./...`, `go vet ./...`, build frontend,
  lint (0 erros/12 avisos conhecidos) e dois testes Playwright de
  painel/PDV também passaram.
- Atenção ao teste: eventos de cancelamento e cálculos tributários
  possuem **gatilhos de imutabilidade**; não é permitido apagá-los
  para executar `go test -count=N` contra **o mesmo banco**.
  `TestNFCeReservation_IsAtomicAndIdempotentPerSale` agora exige
  `TEST_FISCAL_DISPOSABLE_DB=1`, além de `TEST_DATABASE_URL`,
  para impedir execução acidental em ambiente permanente.
  Execute cada repetição num banco de testes independente e descarte
  o banco ao final. Não remova/desative os gatilhos fiscais.
- O teste de integração de persistência usa um builder simulado para
  provar transação/tenant; não substitui a prova com certificado e
  protocolo efetivamente recebidos da SEFAZ. Não usar fixture como
  evidência de autorização fiscal verdadeira.
- Ainda faltam armazenamento integral da resposta SOAP/autenticação
  da fonte para auditoria forense, reconciliação de resposta ambígua
  entre rede/banco, reprocessamento durável, NF-e 55, regras de
  impostos/IBS/CBS por UF, testes de homologação por CNPJ e testes
  físicos de impressão. A issue #33 permanece aberta.

**Estado de segurança:** `NFCE_SEFAZ_PRODUCTION_ENABLED=false` e
`NFCE_SEFAZ_HOMOLOGATION_ENABLED=false` até conclusão dos testes,
configuração real de certificado e aprovação da implantação.
GitHub Actions não foi utilizado.

## Comandos de verificação do arquivo processado

```powershell
# Dentro de um checkout de testes e com Python + lxml instalado:
$env:NFCE_OFFICIAL_VALIDATION_EXPORT_DIR = 'C:\\Temp\\nfce-prova-temporaria'
New-Item -ItemType Directory -Force $env:NFCE_OFFICIAL_VALIDATION_EXPORT_DIR
Push-Location backend
go test ./internal/modules/fiscal/providers/sefaz -run '^TestExportOfficialNFCeSchemaFixtures$' -count=1 -v
Pop-Location
# Faça download do pacote oficial 010f do Portal Nacional NF-e,
# conforme docs/fiscal-2026-auditoria-schema-010f.md.
python scripts/fiscal/validate_official_nfce.py --schema-zip 'C:\\Temp\\nfce-prova-temporaria\\010f.zip' --fixtures-dir $env:NFCE_OFFICIAL_VALIDATION_EXPORT_DIR
```

Testes completos com certificado real, retorno SOAP legítimo, verificação
criptográfica independente do XML arquivado e regras tributárias por UF
**continuam pendentes**; não marcar a issue #33 como resolvida.
