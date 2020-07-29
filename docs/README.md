# Reva Documentation

This documentation is available at: [https://reva.link](https://reva.link)

## Build locally

Before hacking you need to install [Hugo extended version](https://github.com/gohugoio/hugo/releases) and
run `npm install postcss-cli` only if you plan to hack on the theme style.

```
git clone https://github.com/cs3org/reva
cd reva
git submodule update --init --recursive # to install the theme and deps
cd docs
hugo server
```

Open a browser at http://localhost:1313

## Theme
The documentation is based on the [Docsy](https://github.com/google/docsy) theme for technical documentation sites, providing easy site navigation, structure, and more. 
In the [Docsy User Guide](https://www.docsy.dev/docs/getting-started/) to get started.


## Continuous Deployment
If you don't want to build locally, once you create a Pull Request to the repo, a Continuous Integration
step will take care of deploying your changes to Netlify for previewing your changes to the documentation.
