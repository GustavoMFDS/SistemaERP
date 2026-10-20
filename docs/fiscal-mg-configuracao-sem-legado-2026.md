# Configuração fiscal SEF/MG sem pressupor o sistema anterior — 12/10/2026

## Dados confirmados e dados desconhecidos

- Local da operação: **Minas Gerais (MG)**.
- Loja é uma empresa **não MEI**. Isso **não determina** o CRT: Simples Nacional e Regime Normal são situações diferentes que devem ser confirmadas com a contabilidade.
- Cada estabelecimento administra seu próprio CNPJ e dados fiscais: não tratar empresas familiares como filiais do mesmo CNPJ.
- Modelo fiscal utilizado antes do SistemaEmGo: **desconhecido**. Não importar numeração, séries, certificados, configurações de ECF, modelo 2 ou outro emissor por suposição.
- Venda confirmada cria obrigação fiscal; impressão térmica permanece opcional, somente após autorização e com DANFE adequado.

## Regras conservadoras implementadas nesta etapa

1. A interface exige seleção explícita do CRT do emitente. Não deduz CRT=3 de "empresa comum", nem CRT=1 de "não MEI".
2. O preflight local reconhece somente CRT 1, 2, 3 ou 4 **explicitamente registrados**, e jamais diz que todos os dados existem com CRT vazio ou não reconhecido.
3. Como o gateway atual só tem endpoints MG, o preflight e o readiness bloqueiam UF não MG; o código IBGE deve ter sete dígitos e começar em **31** para satisfazer a verificação estrutural local. Isso **não substitui** uma tabela oficial de municípios nem verifica inscrição estadual.
4. O bloqueio apenas impede um diagnóstico enganoso. Não reserva número, não assina e não transmite XML. Estados de preparo continuam `blocked` ou `tax_review_required`, jamais "autorizado".
5. O usuário pode deixar os dados desconhecidos sem preenchimento até confirmação. Dados artificiais ou valores sugeridos pelo software não são usados para liberar a emissão.

## Dados reais a obter antes da homologação

Confirmar com responsável legal/contador: CNPJ, inscrição estadual, regime tributário/CRT, endereço e código IBGE oficial, NCM/CEST/CFOP/CST/CSOSN e regras de produtos e operações, credenciamento NFC-e em MG, série e numeração de início (inclusive continuidade se já emitiu antes), certificado ICP-Brasil adequado e referência segura, informações do CSC quando exigidas pelo leiaute/QR aplicável e demais requisitos vigentes. Sem evidência desses itens e sem testes oficiais, **produção desativada**.

Não é exigido saber agora se o emissor anterior era ECF, papel ou aplicativo; antes do primeiro uso real, apurar modelo/série/numeração e obrigações de substituição com a contabilidade para evitar duplicidades.

## Referências oficiais consultadas em 12/10/2026

- [SEF/MG — Credenciamento NFC-e](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/credenciamento/index.html)
- [SEF/MG — Serviços NFC-e homologação e produção](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/web-services/)
- [SEF/MG — Perguntas frequentes NFC-e](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/Perguntas-Frequentes/respostas_i/index.html)
- [SEF/MG — Downloads e manuais](https://portalsped.fazenda.mg.gov.br/spedmg/nfce/Downloads/index.html)

Nenhuma transmissão/homologação real foi realizada nesta etapa. O suporte aos novos leiautes e notas técnicas precisa ser reavaliado antes da liberação; os testes de XML com casos sintéticos não garantem conformidade tributária geral.
