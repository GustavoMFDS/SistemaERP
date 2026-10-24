# Aceite técnico por terminal — Esterium HTTPS na rede privada

**Estado: pré-lançamento e somente leitura.** Executar este teste em **cada
computador Windows que funcionará como caixa**, a partir de um script
obtido de pacote aprovado. Não altera serviços, firewall, DNS, certificado
do computador, contas, vendas ou banco de dados.

## Antes de começar

1. O servidor Esterium deve estar configurado com um **nome DNS estável
   para a loja** (por exemplo, `erp.minhaloja.local`) e certificado TLS
   válido para aquele nome. O servidor deve expor **somente HTTPS**
   na rede privada; PostgreSQL e Garnet continuam no `127.0.0.1`.
2. A CA emissora deve ser **instalada pelo responsável de TI por
   procedimento independente e aprovado** em cada Windows/Chrome/Edge.
   Este projeto não instala automaticamente certificados raiz nem
   enfraquece a validação TLS do navegador.
3. O operador deve obter, por um **canal distinto**, o SHA-256 do
   certificado DER real do servidor (`ExpectedCertificateSHA256`). Não
   copiar um hash devolvido pela própria conexão sem validá-lo fora dela.
4. Confirmar perfil de rede privada, DNS confiável e firewall do servidor
   restrito à sub-rede da loja, **sem expor PostgreSQL/Garnet**. O teste
   do caixa não inspeciona o firewall do servidor.

## Execução em cada caixa físico

Exemplo conceitual (substituir os marcadores por valores aprovados):

```powershell
.\check-esterium-lan-terminal.ps1 `
  -ERPUrl 'https://erp.minhaloja.local:8443/' `
  -ExpectedCertificateSHA256 '<SHA256_CERTIFICADO_DER_APROVADO>'
```

O utilitário só retorna `PASS` se:
- DNS resolve **exclusivamente** para IPv4 privada RFC1918 ou IPv6 ULA;
  rejeita IP público, loopback, IPv6 link-local e misturas público/privado.
- O HTTPS do servidor utiliza TLS 1.2 ou TLS 1.3, com **cadeia de
  certificação reconhecida pelo Windows**, nome DNS do certificado
  correspondente à URL e SHA-256 igual ao valor aprovado.
- A resposta da página inicial tem status HTTP 200.
- Endereço informado não possui senha/usuário embutido, query, fragmento,
  IP direto, porta de banco/cache ou HTTP inseguro.

Registrar no checklist #151: **nome do caixa, versão do Windows, URL,
resultado PASS/FAIL, data/hora, identificação do operador** (não registrar
credenciais/chaves privadas). Se um dos dois caixas falhar, **bloquear a
liberação LAN** e corrigir DNS, emissor, certificados ou regras de rede
pelo procedimento autorizado, nunca desabilitando TLS.

## Limitações explícitas

A CI executa apenas testes negativos sem rede real. Um `PASS` em um
Windows físico prova alcance e confiança HTTPS **naquele instante e
naquele caixa**, não autoriza uso comercial. Ainda é necessário testar
dois computadores com vendas concorrentes, quedas/reconexão, firewall
do servidor, renovação de certificados, backups SMB/restauração em PC
distinto e aceite operacional. Homologação SEFAZ e impressoras térmicas
não são cobertas aqui.
