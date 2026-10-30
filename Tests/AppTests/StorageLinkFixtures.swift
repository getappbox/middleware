import Foundation

enum StorageLinkFixtures {
    static let json = #"""
{
  "schemaVersion": 1,
  "links": [
    {
      "id": "dropbox-s",
      "provider": "dropbox",
      "sharedURL": "https://www.dropbox.com/s/abc/appinfo.json",
      "installURL": "https://web.getappbox.com?url=/s/abc/appinfo.json",
      "requestURL": "https://install.getappbox.com/appinfo/s/abc/appinfo.json?dl=1",
      "directRequestURL": "https://www.dropbox.com/s/abc/appinfo.json?dl=1",
      "manifestURL": "https://www.dropbox.com/s/abc/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https://install.getappbox.com/install/s/abc/build%201/manifest.plist",
      "ipaURL": "https://www.dropbox.com/s/abc/build%201/App%2BTest.ipa",
      "downloadURL": "https://www.dropbox.com/s/abc/build%201/App%2BTest.ipa?dl=1",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://www.dropbox.com/s/abc/build%201/manifest.plist",
          "ipaFileLink": "https://www.dropbox.com/s/abc/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://www.dropbox.com/s/abc/build%200/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/s/abc/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://www.dropbox.com/s/abc/build%201/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/s/abc/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://www.dropbox.com/s/abc/appinfo.json"
      },
      "browserRequestURL": "https://install.getappbox.com/appinfo/s/abc/appinfo.json",
      "shortenerTarget": "https://web.getappbox.com?url=/s/abc/appinfo.json"
    },
    {
      "id": "dropbox-scl",
      "provider": "dropbox",
      "sharedURL": "https://www.dropbox.com/scl/fi/abc/appinfo.json?rlkey=xyz",
      "installURL": "https://web.getappbox.com?url=/scl/fi/abc/appinfo.json?rlkey=xyz",
      "requestURL": "https://install.getappbox.com/appinfo/scl/fi/abc/appinfo.json?rlkey=xyz&dl=1",
      "directRequestURL": "https://www.dropbox.com/scl/fi/abc/appinfo.json?rlkey=xyz&dl=1",
      "manifestURL": "https://www.dropbox.com/scl/fi/abc/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https://install.getappbox.com/install/scl/fi/abc/build%201/manifest.plist",
      "ipaURL": "https://www.dropbox.com/scl/fi/abc/build%201/App%2BTest.ipa",
      "downloadURL": "https://www.dropbox.com/scl/fi/abc/build%201/App%2BTest.ipa?dl=1",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://www.dropbox.com/scl/fi/abc/build%201/manifest.plist",
          "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://www.dropbox.com/scl/fi/abc/build%200/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://www.dropbox.com/scl/fi/abc/build%201/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://www.dropbox.com/scl/fi/abc/appinfo.json?rlkey=xyz"
      },
      "browserRequestURL": "https://install.getappbox.com/appinfo/scl/fi/abc/appinfo.json?rlkey=xyz",
      "shortenerTarget": "https://web.getappbox.com?url=/scl/fi/abc/appinfo.json?rlkey=xyz"
    },
    {
      "id": "dropbox-encoded",
      "provider": "dropbox",
      "sharedURL": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D",
      "installURL": "https://web.getappbox.com?url=/scl/fi/abc/QA%2520%2526%2520iOS/appinfo.json?rlkey=x%26st=a%252Bb%253D",
      "requestURL": "https://install.getappbox.com/appinfo/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D&dl=1",
      "directRequestURL": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D&dl=1",
      "manifestURL": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https://install.getappbox.com/install/scl/fi/abc/QA%20%26%20iOS/build%201/manifest.plist",
      "ipaURL": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/App%2BTest.ipa",
      "downloadURL": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/App%2BTest.ipa?dl=1",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/manifest.plist",
          "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%200/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/manifest.plist",
            "ipaFileLink": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://www.dropbox.com/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D"
      },
      "browserRequestURL": "https://install.getappbox.com/appinfo/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D",
      "shortenerTarget": "https://web.getappbox.com?url=/scl/fi/abc/QA%20%26%20iOS/appinfo.json?rlkey=x&st=a%2Bb%3D"
    },
    {
      "id": "s3-aws",
      "provider": "s3",
      "sharedURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/appinfo.json",
      "installURL": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fbuilds.s3.eu-west-1.amazonaws.com%2Freleases%2Fappinfo.json",
      "requestURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/appinfo.json",
      "directRequestURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/appinfo.json",
      "manifestURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https%3A%2F%2Fbuilds.s3.eu-west-1.amazonaws.com%2Freleases%2Fbuild%25201%2Fmanifest.plist",
      "ipaURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/App%2BTest.ipa",
      "downloadURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/App%2BTest.ipa",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/manifest.plist",
          "ipaFileLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%200/manifest.plist",
            "ipaFileLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/manifest.plist",
            "ipaFileLink": "https://builds.s3.eu-west-1.amazonaws.com/releases/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://builds.s3.eu-west-1.amazonaws.com/releases/appinfo.json",
        "storageProvider": "s3"
      },
      "browserRequestURL": "https://builds.s3.eu-west-1.amazonaws.com/releases/appinfo.json",
      "shortenerTarget": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fbuilds.s3.eu-west-1.amazonaws.com%2Freleases%2Fappinfo.json"
    },
    {
      "id": "s3-spaces",
      "provider": "s3",
      "sharedURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/appinfo.json",
      "installURL": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fbuilds.nyc3.cdn.digitaloceanspaces.com%2Fios%2Fappinfo.json",
      "requestURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/appinfo.json",
      "directRequestURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/appinfo.json",
      "manifestURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https%3A%2F%2Fbuilds.nyc3.cdn.digitaloceanspaces.com%2Fios%2Fbuild%25201%2Fmanifest.plist",
      "ipaURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/App%2BTest.ipa",
      "downloadURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/App%2BTest.ipa",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/manifest.plist",
          "ipaFileLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%200/manifest.plist",
            "ipaFileLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/manifest.plist",
            "ipaFileLink": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/appinfo.json",
        "storageProvider": "s3"
      },
      "browserRequestURL": "https://builds.nyc3.cdn.digitaloceanspaces.com/ios/appinfo.json",
      "shortenerTarget": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fbuilds.nyc3.cdn.digitaloceanspaces.com%2Fios%2Fappinfo.json"
    },
    {
      "id": "s3-custom",
      "provider": "s3",
      "sharedURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/appinfo.json?channel=a%2Bb&label=100%25&v=2",
      "installURL": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fdownloads.example.test%3A8443%2Freleases%2FQA%2520%2526%2520iOS%2F%25E6%25B5%258B%25E8%25AF%2595%2Fappinfo.json%3Fchannel%3Da%252Bb%26label%3D100%2525%26v%3D2",
      "requestURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/appinfo.json?channel=a%2Bb&label=100%25&v=2",
      "directRequestURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/appinfo.json?channel=a%2Bb&label=100%25&v=2",
      "manifestURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https%3A%2F%2Fdownloads.example.test%3A8443%2Freleases%2FQA%2520%2526%2520iOS%2F%25E6%25B5%258B%25E8%25AF%2595%2Fbuild%25201%2Fmanifest.plist",
      "ipaURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/App%2BTest.ipa",
      "downloadURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/App%2BTest.ipa",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/manifest.plist",
          "ipaFileLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%200/manifest.plist",
            "ipaFileLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/manifest.plist",
            "ipaFileLink": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/appinfo.json?channel=a%2Bb&label=100%25&v=2",
        "storageProvider": "s3"
      },
      "browserRequestURL": "https://downloads.example.test:8443/releases/QA%20%26%20iOS/%E6%B5%8B%E8%AF%95/appinfo.json?channel=a%2Bb&label=100%25&v=2",
      "shortenerTarget": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fdownloads.example.test%3A8443%2Freleases%2FQA%2520%2526%2520iOS%2F%25E6%25B5%258B%25E8%25AF%2595%2Fappinfo.json%3Fchannel%3Da%252Bb%26label%3D100%2525%26v%3D2"
    },
    {
      "id": "s3-percent-key",
      "provider": "s3",
      "sharedURL": "https://cdn.example.test/team%252FQA/100%25/appinfo.json",
      "installURL": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fteam%25252FQA%2F100%2525%2Fappinfo.json",
      "requestURL": "https://cdn.example.test/team%252FQA/100%25/appinfo.json",
      "directRequestURL": "https://cdn.example.test/team%252FQA/100%25/appinfo.json",
      "manifestURL": "https://cdn.example.test/team%252FQA/100%25/build%201/manifest.plist",
      "installURI": "itms-services://?action=download-manifest&url=https%3A%2F%2Fcdn.example.test%2Fteam%25252FQA%2F100%2525%2Fbuild%25201%2Fmanifest.plist",
      "ipaURL": "https://cdn.example.test/team%252FQA/100%25/build%201/App%2BTest.ipa",
      "downloadURL": "https://cdn.example.test/team%252FQA/100%25/build%201/App%2BTest.ipa",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://cdn.example.test/team%252FQA/100%25/build%201/manifest.plist",
          "ipaFileLink": "https://cdn.example.test/team%252FQA/100%25/build%201/App%2BTest.ipa",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://cdn.example.test/team%252FQA/100%25/build%200/manifest.plist",
            "ipaFileLink": "https://cdn.example.test/team%252FQA/100%25/build%200/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://cdn.example.test/team%252FQA/100%25/build%201/manifest.plist",
            "ipaFileLink": "https://cdn.example.test/team%252FQA/100%25/build%201/App%2BTest.ipa",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://cdn.example.test/team%252FQA/100%25/appinfo.json",
        "storageProvider": "s3"
      },
      "browserRequestURL": "https://cdn.example.test/team%252FQA/100%25/appinfo.json",
      "shortenerTarget": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fteam%25252FQA%2F100%2525%2Fappinfo.json"
    },
    {
      "id": "s3-punctuation-query",
      "provider": "s3",
      "sharedURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/appinfo.json?format=a%2Bb&label=100%25&source=dropbox.com",
      "installURL": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Freleases%2F%2523%253F%2526%252B%2521%2527%2528%2529%252A%2Fappinfo.json%3Fformat%3Da%252Bb%26label%3D100%2525%26source%3Ddropbox.com",
      "requestURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/appinfo.json?format=a%2Bb&label=100%25&source=dropbox.com",
      "directRequestURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/appinfo.json?format=a%2Bb&label=100%25&source=dropbox.com",
      "manifestURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/manifest.plist?variant=phone%2Btablet&cache=100%25&select=!'()*",
      "installURI": "itms-services://?action=download-manifest&url=https%3A%2F%2Fcdn.example.test%2Freleases%2F%2523%253F%2526%252B%2521%2527%2528%2529%252A%2Fbuild%25201%2Fmanifest.plist%3Fvariant%3Dphone%252Btablet%26cache%3D100%2525%26select%3D%21%27%28%29%2A",
      "ipaURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/App%2BTest.ipa?version=1&channel=a%2Bb",
      "downloadURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/App%2BTest.ipa?version=1&channel=a%2Bb",
      "metadata": {
        "latestVersion": {
          "name": "QA & iOS <Demo>",
          "identifier": "com.example.demo",
          "version": "1.3",
          "build": "7",
          "manifestLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/manifest.plist?variant=phone%2Btablet&cache=100%25&select=!'()*",
          "ipaFileLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/App%2BTest.ipa?version=1&channel=a%2Bb",
          "timestamp": 1789000000,
          "minosversion": "15.0"
        },
        "versions": [
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.0",
            "build": "1",
            "manifestLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%200/manifest.plist?variant=phone%2Btablet&cache=100%25&select=!'()*",
            "ipaFileLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%200/App%2BTest.ipa?version=1&channel=a%2Bb",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          },
          {
            "name": "QA & iOS <Demo>",
            "identifier": "com.example.demo",
            "version": "1.3",
            "build": "7",
            "manifestLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/manifest.plist?variant=phone%2Btablet&cache=100%25&select=!'()*",
            "ipaFileLink": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/build%201/App%2BTest.ipa?version=1&channel=a%2Bb",
            "timestamp": 1789000000,
            "minosversion": "15.0"
          }
        ],
        "uniqueLinkShared": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/appinfo.json?format=a%2Bb&label=100%25&source=dropbox.com",
        "storageProvider": "s3"
      },
      "browserRequestURL": "https://cdn.example.test/releases/%23%3F%26%2B%21%27%28%29%2A/appinfo.json?format=a%2Bb&label=100%25&source=dropbox.com",
      "shortenerTarget": "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Freleases%2F%2523%253F%2526%252B%2521%2527%2528%2529%252A%2Fappinfo.json%3Fformat%3Da%252Bb%26label%3D100%2525%26source%3Ddropbox.com"
    }
  ],
  "invalidInstallLinks": [
    "https://web.getappbox.com/?provider=box&url=%2Fs%2Fa%2Fappinfo.json",
    "https://web.getappbox.com/?provider=&url=https%3A%2F%2Fcdn.example.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fappinfo.json&url=https%3A%2F%2Fother.example.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3",
    "https://web.getappbox.com/?provider=s3&url=%E0%A4%A",
    "https://web.getappbox.com/?provider=s3&url=http%3A%2F%2Fcdn.example.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fuser%3Apassword%40cdn.example.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fappinfo.json%23fragment",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.example.test%2Fmanifest.plist",
    "https://web.getappbox.com/?provider=s3&url=javascript%3Aalert%281%29",
    "https://web.getappbox.com/?provider=s3&url=%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json%3FX-Amz-Signature%3Dtemporary",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json%3FAWSAccessKeyId%3Dkey%26Signature%3Dsigned",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2F%2561ppinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2Fcdn.test%2Fappinfo.json%2F",
    "https://web.getappbox.com/?provider=s3&url=https%3Acdn.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2F%2Fcdn.test%2Fappinfo.json",
    "https://web.getappbox.com/?provider=s3&url=https%3A%2F%2F%40cdn.test%2Fappinfo.json"
  ]
}
"""#
}
