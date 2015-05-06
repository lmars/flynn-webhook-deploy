Flynn GitHub Webhook Deploy
===========================

Deploy GitHub repositories to Flynn on git push.

Usage
-----

Deploy this Go app to Flynn and set the `REPOS` environment variable to a space
separated list of `repo_name=app_name`, for example:

```
$ git clone https://github.com/lmars/flynn-webhook-deploy.git
$ cd flynn-webhook-deploy
$ flynn create webhook-deploy
$ flynn env set REPOS="lmars/go-flynn-example=go-app lmars/nodejs-flynn-example=nodejs-app"
$ git push flynn master
$ flynn scale web=1
```

The app will now accept GitHub push events to `https://webhook-deploy.$CLUSTER_DOMAIN`
for both the `lmars/go-flynn-example` and `lmars/nodejs-flynn-example` GitHub repos
and deploy them to the `go-app` and `nodejs-app` apps respectively.
