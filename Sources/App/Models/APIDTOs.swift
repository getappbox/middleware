//
//  APIDTOs.swift
//
//
//  Created by Vineet Choudhary on 06/07/26.
//

import Vapor

// MARK: - Error envelope (all /api/v1 endpoints + rate limiting)

struct APIErrorResponse: Content {
	struct Detail: Content {
		let code: String
		let message: String
	}
	let error: Detail

	init(code: String, message: String) {
		self.error = Detail(code: code, message: message)
	}
}

extension APIErrorResponse {
	/// Build a Vapor `Response` carrying the envelope with the given HTTP status.
	static func response(status: HTTPResponseStatus, code: String, message: String) -> Response {
		let response = Response(status: status)
		try? response.content.encode(APIErrorResponse(code: code, message: message))
		return response
	}
}

// MARK: - GET /api/v1/config

struct ConfigResponse: Content {
	let dropboxAppKey: String
}

// MARK: - POST /api/v1/shorten

struct ShortenRequest: Content {
	let url: String
	let name: String
	let version: String
	let build: String
	let identifier: String
	let provider: String?

	init(url: String, name: String, version: String, build: String, identifier: String, provider: String? = nil) {
		self.url = url
		self.name = name
		self.version = version
		self.build = build
		self.identifier = identifier
		self.provider = provider
	}
}

struct ShortenResponse: Content {
	let shortURL: String
	var provider: String? = nil
}

// MARK: - POST /api/v1/mail/send

struct MailSendRequest: Content {
	let name: String
	let version: String
	let build: String
	let to: [String]
	let installURL: String
	let personalMessage: String?
}

struct MailSendResponse: Content {
	let id: String?
}

// MARK: - GET /api/v1/latest-version

struct LatestVersionResponse: Content {
	/// Latest GitHub release version.
	let version: String
	/// The GitHub release page (nil if unavailable).
	let downloadURL: String?
	/// Latest Homebrew cask version, or nil if the cask API was unavailable.
	let homebrewVersion: String?
}

// MARK: - POST /api/v1/notify

struct NotifyRequest: Content {
	/// "slack" or "teams" — selects the incoming-webhook payload envelope.
	let service: String
	/// The user's incoming-webhook URL (must be on an allowlisted host).
	let webhookURL: String
	/// The already-rendered message text.
	let text: String
}
