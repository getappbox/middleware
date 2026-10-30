//
//  ShortLinkController.swift
//
//
//  Created by Vineet Choudhary on 06/07/26.
//

import Vapor

/// `POST /api/v1/shorten` — shorten a supported publication into an appbox.me install link.
struct ShortLinkController: RouteCollection {
	func boot(routes: Vapor.RoutesBuilder) throws {
		routes.post("shorten", use: shorten(req:))
	}

	func shorten(req: Request) async throws -> Response {
		let body: ShortenRequest
		do {
			body = try req.content.decode(ShortenRequest.self)
		} catch {
			return APIErrorResponse.response(status: .badRequest, code: "invalid_body",
											 message: "Expected JSON with url, name, version, build, identifier.")
		}

		do {
			_ = try YOURLSService.targetURL(for: body, base: Environment.shortlinkTargetBase)
			let shortURL = try await req.application.shortLinkProvider.shorten(body, on: req)
			let response = Response(status: .ok)
			try response.content.encode(ShortenResponse(shortURL: shortURL, provider: body.provider == "s3" ? "s3" : nil))
			return response
		} catch let apiError as APIError {
			return APIErrorResponse.response(status: apiError.status, code: apiError.code, message: apiError.message)
		}
	}
}
