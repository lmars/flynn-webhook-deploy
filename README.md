Flynn GitHub Webhook Deploy
===========================

Deploy GitHub repositories to Flynn on git push.

Usage
-----

Deploy this Go app to Flynn with a Postgres DB:

```
$ git clone https://github.com/lmars/flynn-webhook-deploy.git
$ cd flynn-webhook-deploy
$ flynn create webhook-deploy
$ flynn resource add postgres
$ git push flynn master
$ flynn scale web=1
```

You can now add repos by going to `https://webhook-deploy.$CLUSTER_DOMAIN` in
your browser, or using psql from the command line:

```
flynn pg psql -- -c "INSERT INTO repos (name, app) VALUES ('lmars/go-flynn-example', 'go-app')"
```

With a repo added, this app will now accept GitHub push events to
`https://webhook-deploy.$CLUSTER_DOMAIN` for repos in the db and deploy them
to the corresponding app (e.g. in the example above, push events for
`lmars/go-flynn-example` will be deployed to the `go-app` Flynn app).
