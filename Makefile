phony: deploy

deploy: cd infrastructure && npx cdk deploy
