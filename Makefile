phony: deploy

deploy:
	cd infrastructure && npx cdk deploy
bootstrap:
	cd infrastructure && npx cdk bootstrap
synth:
	cd infrastructure && npx cdk synth
