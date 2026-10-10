# Auditoria de aderência fiscal NF-e/NFC-e (10/10/2026)

**Resultado:** os dois cenários sintéticos de NFC-e 65 assinada (tributação legada e IBS/CBS regular) agora passam na validação estrutural **XSD 010f** oficial, sem transmissão SEFAZ. Isso NÃO equivale a homologação, autorização ou validade tributária de qualquer venda real.

## Origem e rastreabilidade do schema

- Portal nacional: https://www.nfe.fazenda.gov.br/portal/listaConteudo.aspx?tipoConteudo=BMPFMBoln3w%3D
- ZIP oficial publicado em 31/08/2026: https://www.nfe.fazenda.gov.br/portal/exibirArquivo.aspx?conteudo=8ITFuBLltXs%3D
- Pacote: `PL_010f_v1.04`, NT 2025.002 v1.50 e NT 2026.007 v1.00.
- SHA-256 confirmado no ZIP baixado: `B8589490A58A09A993A80E6AC4D7ED10F20892061ECFC56719337098D4B95998`.
- Entrypoint: `nfe_v4.00.xsd`; dependências: `leiauteNFe_v4.00.xsd`, `tiposBasico_v4.00.xsd`, `DFeTiposBasicos_v1.00.xsd` e `xmldsig-core-schema_v1.01.xsd`.

## Defeitos reais corrigidos após comparação com o XSD

1. **Assinatura em posição inválida.** `TNFe` exige `infNFe`, `infNFeSupl` (se existir) e **depois** `ds:Signature`. O assinador anterior colocava `ds:Signature` antes do suplemento.
2. **Validação prematura.** O XSD de `NFe` exige assinatura, mas o serviço tentava validar o documento *antes* de assiná-lo. Agora assina, valida o XML assinado e só então armazena; um erro do XSD bloqueia persistência.
3. **Total IBS/CBS fora de ordem.** Em `IBSCBSTot/gCBS`, a sequência exigida é `vDif`, `vDevTrib`, `vCBS`, `vCredPres`, `vCredPresCondSus`. O antigo gerador começava pelos créditos.

## Procedimento de prova reproduzível

Em PowerShell, na raiz do repositório, com Go e Python/lxml disponíveis:

```powershell
$fixtures = Join-Path (Get-Location) '.nfce-schema-proof'
New-Item -ItemType Directory -Force $fixtures | Out-Null
$env:NFCE_OFFICIAL_VALIDATION_EXPORT_DIR = $fixtures
Push-Location backend
go test ./internal/modules/fiscal/providers/sefaz -run '^TestExportOfficialNFCeSchemaFixtures$' -count=1 -v
Pop-Location
Invoke-WebRequest -UseBasicParsing 'https://www.nfe.fazenda.gov.br/portal/exibirArquivo.aspx?conteudo=8ITFuBLltXs%3D' -OutFile (Join-Path $fixtures '010f.zip')
python -m pip install lxml
python scripts/fiscal/validate_official_nfce.py --schema-zip (Join-Path $fixtures '010f.zip') --fixtures-dir $fixtures
```

O teste Go exporta dois XMLs reais gerados pelo builder e assinados com **certificado RSA efêmero**, além dos dois candidatos não assinados. O script valida o SHA-256 do ZIP oficial, carrega todos os cinco XSDs, exige os documentos assinados válidos e os não assinados inválidos. Não usa certificado, CNPJ ou transações reais.

## O que o XSD NÃO comprova

- Aceite pela SEFAZ (`cStat=100`), validade cadastral CNPJ/IE, certificado A1 autorizado ou endpoints de UF. Não se enviou nenhum documento.
- Cálculo fiscal correto de ICMS, PIS/COFINS, IBS/CBS e classificações por UF, CRT, CFOP, NCM, regime, data e produto. O XSD verifica estrutura, não todas as regras de negócio e tabelas oficiais.
- **NFC-e do varejo:** o catálogo de serviços está preparado somente para MG; o builder restringe CFOP 5xxx, cliente não identificado e pagamentos dinheiro/Pix/transferência. Cartões e emissão a destinatário identificado ainda não são implementados nesse perfil.
- **NF-e modelo 55:** o gerador legado `GenerateNFeXML` não possui pipeline comprovado de autorização, eventos e entrega. Não declarar que o modelo 55 está pronto.
- **Arquivo processado:** o repositório arquiva XML `NFe` assinado e campos de resultado, mas não materializa o `nfeProc` com `protNFe` genuíno da SEFAZ; não oferecer esse XML isolado como documento fiscal processado final.
- Tratamento de duplicidade, rejeição e timeout de envio, fila de execução automática, numeração/eventos, contingência legal, certificado e homologação real por CNPJ.

**Bloqueio:** manter transmissão de produção desabilitada e a issue #32 aberta. Mesmo com XSD válido nesses dois casos, o sistema **ainda não** emite documentos fiscais oficiais automaticamente por venda.
