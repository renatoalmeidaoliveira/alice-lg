# Filtros e busca

## Filtros disponíveis

| Filtro | Exemplo | Descrição |
|---|---|---|
| Família de endereços | IPv4 / IPv6 | Mostra só uma das famílias |
| MPLS Label | `16001` | Rotas com qualquer uma das labels escolhidas |
| BGP Communities | `rt:64512:20` | Rotas com o route target escolhido |

## Busca global

A busca aceita prefixos, nomes de vizinhos, ASNs e labels MPLS:

```
198.51.100.0/24
198.51.100 label:24002
label:16001
```

> O RD de cada rota VPN aparece logo abaixo do prefixo.

Voltar para a [introdução](01-introducao.md).
