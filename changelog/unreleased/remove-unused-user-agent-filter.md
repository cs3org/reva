Change: We removed the unused `allowed_user_agents` config option

It was not used anywhere in the code, but three places had the config option. Dropping it before it spreads further.

https://github.com/cs3org/reva/pull/2338
