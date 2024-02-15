
/* jshint ignore:start */
const https = require('https');
const fs = require('fs');
const url = require('url');
const fetch = require('node-fetch');
const { isNativeError } = require('util/types');

const SERVER_NAME = process.env.HOST || 'meshdir';
const SERVER_HOST = `${SERVER_NAME}.docker`;
const SERVER_ROOT = `https://${SERVER_HOST}`;
const USER = `einstein`;
const PROVIDER_ID = SERVER_HOST;
const MESH_PROVIDER = SERVER_HOST;

// const HTTPS_OPTIONS = {
//   key: fs.readFileSync(`/etc/letsencrypt/live/${SERVER_HOST}/privkey.pem`),
//   cert: fs.readFileSync(`/etc/letsencrypt/live/${SERVER_HOST}/cert.pem`),
//   ca: fs.readFileSync(`/etc/letsencrypt/live/${SERVER_HOST}/chain.pem`)
// }
const HTTPS_OPTIONS = {
  key: fs.readFileSync(`/tls/${SERVER_NAME}.key`),
  cert: fs.readFileSync(`/tls/${SERVER_NAME}.crt`)
}

function sendHTML(res, text) {
  res.end(`<!DOCTYPE html><html><head></head><body>${text}</body></html>`);
}

// singleton global, naively assume only one share exists at a time:
let mostRecentShareIn = {};

async function getServerConfig(otherUser) {
  console.log('getServerConfig', otherUser);

  let otherServer = otherUser.split('@').splice(1).join('@').replace('\/', '/');
  console.log(otherServer);
  if (otherServer.startsWith('http://')) {
    // support http:// for testing
  } else if (!otherServer.startsWith('https://')) {
    otherServer = `https://${otherServer}`;
  }
  if (!otherServer.endsWith('/')) {
    otherServer = `${otherServer}/`;
  }
  console.log('fetching', `${otherServer}ocm-provider/`);
  const configResult = await fetch(`${otherServer}ocm-provider/`);
// const text = await configResult.text();
// console.log({ text });
// JSON.parse(text);
  return { config: await configResult.json(), otherServer };
}

async function notifyProvider(obj, notif) {
  console.log('notifyProvider', obj, notif);
  // FIXME: reva sets no `sharedBy` and no `sender`
  // and sets `owner` to a user opaqueId only (e.g. obj.owner: '4c510ada-c86b-4815-8820-42cdf82c3d51').
  // what we ultimately need when a share comes from reva is obj.meshProvider, e.g.: 'revad1.docker'.
  const { config } = await getServerConfig(obj.sharedBy || obj.sender || /* obj.owner || */ `${obj.owner}@${obj.meshProvider}`);
  if (config.endPoint.substr(-1) == '/') {
    config.endPoint = config.endPoint.substring(0, config.endPoint.length - 1);
  }

  const postRes = await fetch(`${config.endPoint}/notifications`, {
    method: 'POST',
    body: JSON.stringify(notif)
  });
  console.log('notification sent!', postRes.status, await postRes.text());
}

async function forwardInvite(invite) {
  console.log('forwardInvite', invite);
  const { config, otherServer } = await getServerConfig(invite);
  console.log('discovered', config, otherServer);
  if (!config.endPoint) {
    config.endPoint = process.env.FORCE_ENDPOINT;
  }

  const inviteSpec = {
    invite: {
      token: invite.split('@')[0],
      userId: 'marie',
      recipientProvider: 'stub2.docker',
      name: 'Marie Curie',
      email: 'marie@cesnet.cz',
    }
  }
  let endPoint = config.endPoint || config.endpoint;
  if (endPoint.substr(-1) == '/') {
    endPoint = endPoint.substring(0, endPoint.length - 1);
  }
  console.log('posting', `${endPoint}/invites/accept`, JSON.stringify(inviteSpec, null, 2))
  const postRes = await fetch(`${endPoint}/invites/accept`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(inviteSpec, null, 2),
  });
  console.log('invite forwarded', postRes.status, await postRes.text());
}
async function createShare(consumer) {
  console.log('createShare', consumer);
  const { config, otherServer } = await getServerConfig(consumer);
  console.log(config);
  if (!config.endPoint) {
    config.endPoint = process.env.FORCE_ENDPOINT;
  }

  const shareSpec = {
    shareWith: 'marie', // consumer,
    name: 'Test share from stub',
    providerId: PROVIDER_ID,
    meshProvider: MESH_PROVIDER,
    owner: USER,
    ownerDisplayName: USER,
    sender: `${USER}@${SERVER_HOST}`,
    senderDisplayName: USER,
    shareType: 'user',
    resourceType: 'file',
    // see https://github.com/cs3org/ocm-test-suite/issues/25#issuecomment-852151913
    protocol: JSON.stringify({ name: 'webdav', options: { token: 'shareMe' } }) // sic.
  }
  console.log(shareSpec, shareSpec.protocol);
  if (config.endPoint.endsWith('/')) {
    config.endPoint = config.endPoint.substring(0, config.endPoint.length - 1);
  }

  const postRes = await fetch(`${config.endPoint}/shares`, {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json'
    },
    body: JSON.stringify(shareSpec, null, 2),
  });
  console.log('outgoing share created!', postRes.status, await postRes.text());
  return otherServer;
}
const server = https.createServer(HTTPS_OPTIONS, async (req, res) => {
  console.log(req.method, req.url, req.headers);
  let bodyIn = '';
  req.on('data', (chunk) => {
    console.log('CHUNK', chunk.toString());
    bodyIn += chunk.toString();
  });
  req.on('end', async () => {
    try {
      if (req.url === '/ocm-provider/') {
        console.log('yes /ocm-provider/');
        res.end(JSON.stringify({
          enabled: true,
          apiVersion: '1.0-proposal1',
          endPoint: `${SERVER_ROOT}/ocm`,
          resourceTypes: [
            {
              name: 'file',
              shareTypes: [ 'user', 'group' ],
              protocols: { webdav: '/webdav/' }
            }
          ]
        }));
      } else if (req.url === '/ocm/shares') {
        console.log('yes /ocm/shares');
        try {
          mostRecentShareIn = JSON.parse(bodyIn);
        } catch (e) {
          res.writeHead(400);
          sendHTML(res, 'Cannot parse JSON');
        }
        // {
        //   shareWith: "admin@https:\/\/stub1.pdsinterop.net",
        //   shareType: "user",
        //   name: "Reasons to use Nextcloud.pdf",
        //   resourceType: "file",
        //   description:"",
        //   providerId:202,
        //   owner: "alice@https:\/\/nc1.pdsinterop.net\/",
        //   ownerDisplayName: "alice",
        //   sharedBy: "alice@https:\/\/nc1.pdsinterop.net\/",
        //   sharedByDisplayName":"alice",
        //   "protocol":{
        //     "name":"webdav",
        //     "options":{
        //       "sharedSecret":"lvns5N9ZXm1T1zx",
        //       "permissions":"{http:\/\/open-cloud-mesh.org\/ns}share-permissions"
        //     }
        //   }
        // }
        // obj.id = obj.providerId;
        res.writeHead(201, {
          'Content-Type': 'application/json'
        });
        res.end(JSON.stringify({
          "recipientDisplayName": "Marie Curie"
        }, null, 2));
      } else if (req.url.startsWith('/publicLink')) {
        console.log('yes publicLink');
        const urlObj = new URL(req.url, SERVER_ROOT);
        if (urlObj.search.startsWith('?saveTo=')) {
          console.log('creating share', urlObj.search);
          const otherServerRoot = await createShare(decodeURIComponent(urlObj.search).substring('?saveTo='.length));
          res.writeHead(301, {
            location: otherServerRoot
          });
          sendHTML(res, `Redirecting you to ${otherServerRoot}`);
        } else {
          sendHTML(res, 'yes publicLink, saveTo?');
        }
      } else if (req.url.startsWith('/forwardInvite')) {
        console.log('yes forwardInvite');
        const urlObj = new URL(req.url, SERVER_ROOT);
        await forwardInvite(decodeURIComponent(urlObj.search).substring('?'.length));
        sendHTML(res, 'yes forwardInvite');
      } else if (req.url.startsWith('/shareWith')) {
        console.log('yes shareWith');
        const urlObj = new URL(req.url, SERVER_ROOT);
        await createShare(decodeURIComponent(urlObj.search).substring('?'.length));
        sendHTML(res, 'yes shareWith');
      } else if (req.url.startsWith('/acceptShare')) {
        console.log('yes acceptShare');
        try {
          console.log('Creating notif to accept share, obj =', mostRecentShareIn);
          const notif = {
            type: 'SHARE_ACCEPTED',
            resourceType: mostRecentShareIn.resourceType,
            providerId: mostRecentShareIn.providerId,
            notification: {
              sharedSecret: (
                mostRecentShareIn.protocol ?
                (
                  mostRecentShareIn.protocol.options ?
                  mostRecentShareIn.protocol.options.sharedSecret :
                  undefined
                ) :
                undefined
              ),
              message: 'Recipient accepted the share'
            }
          };
          notifyProvider(mostRecentShareIn, notif);
        } catch (e) {
          console.error(e);
          sendHTML(res, `no acceptShare - fail`);
        }
        sendHTML(res, 'yes acceptShare');
      } else if (req.url.startsWith('/deleteAcceptedShare')) {
        console.log('yes deleteAcceptedShare');
        const notif = {
          type: 'SHARE_DECLINED',
          message: 'I don\'t want to use this share anymore.',
          id: mostRecentShareIn.id,
          createdAt: new Date()
        };
        // When unshared from the provider side:
        // {
        //   "notificationType":"SHARE_UNSHARED",
        //   "resourceType":"file",
        //   "providerId":"89",
        //   "notification":{
        //     "sharedSecret":"N7epqXHRKXWbg8f",
        //     "message":"File was unshared"
        //   }
        // }
        console.log('deleting share', mostRecentShareIn);
        try {
          notifyProvider(mostRecentShareIn, notif);
        } catch (e) {
          sendHTML(res, `no deleteAcceptedShare - fail ${provider}ocm-provider/`);
        }
        sendHTML(res, 'yes deleteAcceptedShare');
      } else if (req.url == '/') {
        console.log('yes a/', mostRecentShareIn);
        sendHTML(res, 'yes /' + JSON.stringify(mostRecentShareIn, null, 2));
      } else if (req.url.startsWith('/meshdir?')) {

    const queryObject = url.parse(req.url, true).query;
    console.log(queryObject);
        const config = {
          nextcloud1: "https://nextcloud1.docker/index.php/apps/sciencemesh/accept",
          owncloud1: "https://owncloud1.docker/index.php/apps/sciencemesh/accept",
          cernbox1: "https://cernbox1.docker/sciencemesh-app/invitations",
          nextcloud2: "https://nextcloud2.docker/index.php/apps/sciencemesh/accept",
          owncloud2: "https://owncloud2.docker/index.php/apps/sciencemesh/accept",
          cernbox2: "https://cernbox2.docker/sciencemesh-app/invitations",
          stub2: "https://stub.docker/ocm/invites/forward",
        };
        const items = [];
        const scriptLines = [];
        Object.keys(config).forEach(key => {
          if (typeof config[key] === "string") {
            items.push(`  <li><a id="${key}">${key}</a></li>`);
            scriptLines.push(`  document.getElementById("${key}").setAttribute("href", "${config[key]}"+window.location.search);`);
          } else {
            const params = new URLSearchParams(req.url.split('?')[1]);
		  console.log(params);
            const token = params.get('token');
            const providerDomain = params.get('providerDomain');
            items.push(`  <li>${key}: Please run <tt>ocm-invite-forward -idp ${providerDomain} -token ${token}</tt> in Reva's CLI tool.</li>`);
          }
        })

        console.log('meshdir', mostRecentShareIn);
        sendHTML(res, `Welcome to the meshdir stub. Please click a server to continue to:\n<ul>${items.join('\n')}</ul>\n<script>\n${scriptLines.join('\n')}\n</script>\n`);
      } else {
        console.log('not recognized');
        sendHTML(res, 'OK');
      }
    } catch (e) {
      console.error(e);
    }
  });
});
server.listen(443);
/* jshint ignore:end */
