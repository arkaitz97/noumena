## 1. Deploy Woodpecker via Coolify UI

- [ ] 1.1 In Coolify UI, create a new **Docker Compose** service and paste the compose file from `design.md`
- [ ] 1.2 Generate a random agent secret: `openssl rand -base64 32`
- [ ] 1.3 Register a GitHub OAuth App at https://github.com/settings/applications/new (Homepage: Woodpecker URL, Callback: `http://<vm-ip>:8000/authorize`)
- [ ] 1.4 Set `WOODPECKER_GITHUB_CLIENT_ID`, `WOODPECKER_GITHUB_CLIENT_SECRET`, and `WOODPECKER_AGENT_SECRET` in Coolify UI → Environment Variables
- [ ] 1.5 Click **Deploy** in Coolify and wait for the Woodpecker containers to start
- [ ] 1.6 Verify the Woodpecker UI is accessible (via Coolify proxy domain or `http://<vm-ip>:8000`)
- [ ] 1.7 Log in via GitHub OAuth and confirm the agent shows as connected

## 2. Configure GitHub webhook

- [ ] 2.1 In Woodpecker UI, add the noumena repo and copy the webhook URL
- [ ] 2.2 Go to GitHub repo → Settings → Webhooks → Add webhook, paste Woodpecker URL, content-type `application/json`, trigger: "Just the push event"
- [ ] 2.3 Push a test commit to `main` and verify Woodpecker picks it up (pipeline shows pending then running)

## 3. Create the CI pipeline config

- [x] 3.1 Create `.woodpecker.yml` at repo root with the `build-and-push` pipeline
- [x] 3.2 Add step: `docker buildx build --push -t registry.arkaitzmolinuevo.com/noumena:<tags>`
- [x] 3.3 Implement tag logic: `:staging` + `:main-<sha>` on main, `:<version>` + `:production` on tags
- [x] 3.4 Add step: `curl -X POST` to Coolify webhook URL on success
- [ ] 3.5 Add registry credentials as Woodpecker secrets (`REGISTRY_USERNAME`, `REGISTRY_PASSWORD`)
- [ ] 3.6 Add webhook URLs as Woodpecker secrets (`COOLIFY_STAGING_WEBHOOK`, `COOLIFY_PRODUCTION_WEBHOOK`)

## 4. Remove GitHub Actions workflow

- [x] 4.1 Delete `.github/workflows/docker-build.yml`
- [x] 4.2 Remove the `COOLIFY_STAGING_WEBHOOK` and `COOLIFY_PRODUCTION_WEBHOOK` GitHub secrets (no longer used by GitHub)

## 5. Update documentation

- [x] 5.1 Update `docs/deploy-coolify.md` to reference Woodpecker instead of GitHub Actions
- [x] 5.2 Update `docs/setup-guide.md` to reference Woodpecker for CI/CD

## 6. Verify end-to-end

- [ ] 6.1 Push a commit to `main`, verify Woodpecker builds and pushes `:staging` to registry, Coolify staging redeploys
- [ ] 6.2 Push `git tag v9.9.9-test && git push origin v9.9.9-test`, verify Woodpecker builds `:v9.9.9-test` + `:production`, Coolify production redeploys
- [ ] 6.3 Delete the test tag: `git tag -d v9.9.9-test && git push origin --delete v9.9.9-test`
