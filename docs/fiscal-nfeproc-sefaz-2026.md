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
- O schema 010f oficial anteriormente foi usado para **NFe assinada**,
  não ainda para o `nfeProc` desta etapa. A validação do arquivo
  processado completo contra `procNFe_v4.00.xsd`, com dependências
  oficiais atualizadas, permanece **pendente**.
- A migração 0040, nova rota, transação e teste integrado de persistência
  foram implementados, **mas ainda não passaram na bateria final**
  desta etapa: o computador conectado ficou indisponível.
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
