//
//  YOURLSService.swift
//
//
//  Created by Vineet Choudhary on 06/07/26.
//

import Vapor
import Crypto

/// Seam for the short-link provider so tests can stub it.
protocol ShortLinkProviding {
	func shorten(_ request: ShortenRequest, on req: Request) async throws -> String
}

extension Application {
	private struct ShortLinkProviderKey: StorageKey {
		typealias Value = ShortLinkProviding
	}

	var shortLinkProvider: ShortLinkProviding {
		get { storage[ShortLinkProviderKey.self] ?? YOURLSService() }
		set { storage[ShortLinkProviderKey.self] = newValue }
	}
}

/// Shortens install links via the appbox.me YOURLS instance.
struct YOURLSService: ShortLinkProviding {
	/// Mirrors TinyURL.m's per-process auth token (an opaque UUID salted per request).
	private static let processAuthToken = UUID().uuidString

	private struct YOURLSForm: Content {
		let action: String
		let format: String
		let url: String
		let userAuthToken: String
		let hash: String
		let timestamp: String
		let signature: String
		let title: String
	}

	func shorten(_ request: ShortenRequest, on req: Request) async throws -> String {
		guard let secret = Environment.yourlsSignatureSecret, !secret.isEmpty else {
			throw APIError(status: .internalServerError, code: "misconfigured", message: "Shortener is not configured.")
		}

		let target = try Self.targetURL(for: request, base: Environment.shortlinkTargetBase)

		let timestamp = String(Int(Date().timeIntervalSince1970))
		let form = YOURLSForm(
			action: "shorturl",
			format: "json",
			url: target,
			userAuthToken: Self.sha512Hex(timestamp + Self.processAuthToken),
			hash: "sha512",
			timestamp: timestamp,
			signature: Self.sha512Hex(timestamp + secret),
			title: "\(request.name), \(request.identifier), \(request.version), \(request.build)")

		var lastFailure = "Shortener unavailable."
		for attempt in 1...3 {
			let response: ClientResponse
			do {
				response = try await req.client.post(URI(string: Environment.yourlsAPIURL)) { clientRequest in
					try clientRequest.content.encode(form, as: .urlEncodedForm)
				}
			} catch {
				lastFailure = "Shortener request failed: \(error)"
				continue
			}

			let shortURL = try? response.content.get(String.self, at: "shorturl")
			switch Self.outcome(status: response.status.code, shortURL: shortURL) {
			case .success(let url):
				return url
			case .permanentFailure:
				throw APIError(status: .badGateway, code: "shortener_unavailable",
							   message: "The shortener rejected the URL.")
			case .retryable:
				lastFailure = "Shortener responded \(response.status.code) (attempt \(attempt))."
			}
		}
		throw APIError(status: .badGateway, code: "shortener_unavailable", message: lastFailure)
	}

	enum AttemptOutcome: Equatable {
		case success(String)
		case permanentFailure
		case retryable
	}

	/// How to treat one YOURLS response.
	static func outcome(status: UInt, shortURL: String?) -> AttemptOutcome {
		if let shortURL, !shortURL.isEmpty {
			return .success(shortURL)
		}
		return status == 400 ? .permanentFailure : .retryable
	}

	static func targetURL(for request: ShortenRequest, base: String) throws -> String {
		switch request.provider {
		case nil, "dropbox": return try targetURL(forShareURL: request.url, base: base)
		case "s3": return try PublicInstallLink.validate(request.url, base: base)
		default: throw APIError(status: .badRequest, code: "invalid_provider", message: "Unsupported storage provider.")
		}
	}

	/// Builds the installer web-page URL wrapping the Dropbox share path.
	static func targetURL(forShareURL shareURL: String, base: String) throws -> String {
		guard let range = shareURL.range(of: "dropbox.com") else {
			throw APIError(status: .badRequest, code: "invalid_url", message: "Expected a dropbox.com share URL.")
		}
		return "\(base)?url=\(String(shareURL[range.upperBound...]))"
	}

	/// Lowercase hex SHA-512.
	static func sha512Hex(_ input: String) -> String {
		SHA512.hash(data: Data(input.utf8)).map { String(format: "%02x", $0) }.joined()
	}
}
