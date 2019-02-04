# REVA

Cloud Storage Sync & Share Interoperability Platform

# Getting started - Docker

```
git clone https://github.com/cernbox/reva
cd reva
docker build . -t revad
docker run -p 9999:9999 -p 9998:9998 -d revad
```

# Getting started - Docker with your own config

```
git clone https://github.com/cernbox/reva
cd reva
docker build . -t revad
docker run -p 9999:9999 -p 9998:9998 -v /your/config:/etc/revad/revad.toml -d revad
```
