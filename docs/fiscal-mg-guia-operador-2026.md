# Guia de configuração fiscal na interface (MG) — 12/10/2026

## Experiência implementada

A página **Fiscal → Nota fiscal da loja** apresenta um `FiscalSetupGuide` com seis passos recolhíveis, conteúdo acessível por teclado e atalhos para abrir as seções existentes de cadastro do emitente, certificado e pendências. URLs externas abrem em nova aba, usando `noopener noreferrer`. A opção de imprimir permanece separada da autorização.

Não há persistência de checkboxes nem campo para usuário/senha do governo. Os indicadores são **somente de preenchimento local** e usam o `readiness` já existente para mostrar "dados básicos registrados; conferir", "referência A1 cadastrada; não validada" e similares. Credenciamento da SEF/MG, enquadramento tributário, numeração/série inicial e homologação oficial continuam exigindo confirmação externa. Nenhuma indicação representa autorização de nota ou ativação da transmissão.

## Como o operador segue o guia

1. Reúne CNPJ e dados cadastrais por consulta pública da Receita; verifica IE via cadastro de contribuintes MG; consulta opção pelo Simples Nacional e código de município no IBGE; pede CRT e regras aplicáveis ao contador. O sistema não infere CRT do fato de a empresa não ser MEI.
2. O responsável autorizado acessa o **SIARE**, pela opção de login do tipo de usuário aplicável (por exemplo, Inscrição Estadual, CPF do responsável e senha pessoal) ou por certificado digital. O responsável ou contador autorizado confere/solicita credenciamento de NFC-e modelo 65. **Esse login não ocorre no PDV.**
3. O emissor providencia certificado ICP-Brasil vinculado ao CNPJ e solicita provisionamento seguro de A1 no servidor; a interface armazena somente referência fornecida pelo técnico. CSC e ID Token podem ser consultados no SIARE: `HOME > DOCUMENTOS ELETRÔNICOS > MANUTENÇÃO CADASTRO EMISSOR`, pesquisar pela IE e selecionar Atualizar. O QR Code versão 3 dispensa CSC segundo NT 2025.001, mas a SEF/MG ainda descreve fornecimento do CSC em seu credenciamento e as versões anteriores podem usá-lo. Não pedir ao operador que exponha o segredo em campo de referência.
4. Contador revisa NCM, CEST quando aplicável, CFOP, ICMS CST/CSOSN, PIS/COFINS, IBS/CBS, meios de pagamento e fluxo de devolução. Os contadores locais de produto com perfil e NCM são **checagem de cadastro**, não classificação tributária certificada.
5. Técnico configura apenas o **ambiente de homologação** e confirma série/numeração e se houve emissão anterior de NFC-e noutro software; modelo fiscal anterior não é assumido. O formulário existente continua sem ligar transmissão.
6. Depois de credenciamento, dados reais e disponibilidade do executor fiscal: validar comunicação/consulta com SEF/MG, assinatura, rejeições, timeout, nfeProc e impressão térmica opcional. NF-e modelo 55 mantém credenciamento e processo separados.

## Fontes oficiais

- [Receita Federal — CNPJ](https://www.gov.br/pt-br/servicos/consultar-cadastro-nacional-de-pessoas-juridicas)
- [Receita Federal — consulta optantes Simples](https://www8.receita.fazenda.gov.br/SIMPLESNACIONAL/Servicos/Grupo.aspx?grp=9)
- [Cadastro de contribuintes / IE MG](https://portalsped.fazenda.mg.gov.br/spedmg/nfe/Consulta-Cadastro-de-Contribuintes/)
- [Códigos de municípios IBGE](https://www.ibge.gov.br/explica/codigos-dos-municipios.php)
- [SEF/MG — credenciamento NFC-e](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/credenciamento/index.html)
- [SEF/MG — SIARE](https://siare.fazenda.mg.gov.br/sicaf/)
- [SEF/MG — dúvidas sobre certificado, CSC e ID Token](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/Perguntas-Frequentes/respostas_i/index.html)
- [SEF/MG — endpoints oficiais](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/web-services/)
- [Portal Nacional NF-e — NT 2025.001 v1.03 (QR Code v3)](https://www.nfe.fazenda.gov.br/Portal/exibirArquivo.aspx?conteudo=NvuzQGYd6E8%3D)
- [SEF/MG — credenciamento NF-e 55](https://portalsped.fazenda.mg.gov.br/spedmg/nfe/credenciamento/)

## Regras de segurança e limites

- **Nunca** solicitar/gravar login e senha SIARE, senha PFX/A1, conteúdo PFX, chave privada ou CSC dentro do guia ou por suporte informal.
- Um certificado/CSC referenciado não confirma identidade criptográfica nem credenciamento.
- A UI é somente educativa, não submete solicitações nos portais oficiais e não liga SEFAZ.
- As informações e normas podem mudar; consultar as fontes oficiais e o contador antes do uso real.
- Teste de navegador com API simulada assegura renderização, URL de consulta oficial e abertura dos campos internos; não substitui E2E com backend real ou homologação fiscal.
