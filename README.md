# collect-repos 

## Install

```bash

git clone http://github.com/zoroqi/collect-repos
go install / go build
```

## Usage

```bash
Usage of ./collect-repos:
  -branch string
    	commit branch
  -commitAuthor string
    	commit author (default "github-actions[bot]")
  -commitEmail string
    	commit email (default "41898282+github-actions[bot]@users.noreply.github.com")
  -config string
    	config
  -file string
    	commit file name
  -license string
    	license name, default username
  -repository string
    	update repository
  -token string
    	github token
  -username string
    	github username
```

config example

```yaml
- name: zoroqi
  userType: user
  file: README.md
- name: google
  userType: org
  file: google.md
- name: apple
  userType: org
  file: apple.md

# userType: user or org
# user: collect starred
# org: collect repository
```

## Demo

```bash
./collect-repos --username zoroqi > zoroqi-starred.md
```


- [`zoroqi/my-awesome`](https://github.com/zoroqi/my-awesome)
- [update awesome-stars every day by GitHub Action](https://github.com/zoroqi/my-awesome/blob/master/.github/workflows/schedules.yml) the example with GitHub Action

