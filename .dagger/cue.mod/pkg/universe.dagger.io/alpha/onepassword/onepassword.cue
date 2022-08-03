package onepassword

import (
	"dagger.io/dagger"
)

#Credentials: {
	// The sign-in address for the account
	address: dagger.#Secret

	// The email adress associated with the account
	email: dagger.#Secret

	// The secret key
	secretKey: dagger.#Secret

	// The password
	password: dagger.#Secret
}

#Reference: {
	// The vault name
	vault: string

	// The item name
	item: string

	// The section label (optional)
	section: string | *""

	// The field label
	field: string

	output: "op://\(vault)/\(item)/\(section)/\(field)"
}

_#Version: "2"
