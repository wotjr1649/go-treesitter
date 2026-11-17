import Foundation

struct Rental: Codable {
    let id: Int
    let customer: String
    var returnDate: Date?
}

enum RentalError: Error {
    case notFound(Int)
}

final class RentalStore {
    private var rentals: [Int: Rental] = [:]

    func open() -> [Rental] {
        return rentals.values.filter { $0.returnDate == nil }.sorted { $0.id < $1.id }
    }

    func close(_ id: Int, at date: Date = Date()) throws {
        guard var rental = rentals[id] else { throw RentalError.notFound(id) }
        rental.returnDate = date
        rentals[id] = rental
    }
}
