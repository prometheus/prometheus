//Deprecated: in favor of universe.dagger.io/alpha package
package discord

// API: https://discord.com/developers/docs/resources/webhook#execute-webhook
#Parameter: {
	content:     string
	username?:   string
	avatar_url?: string
	tts?:        bool
	embeds?: [...#Embed]
	// // not implemented
	// allowed_mentions?: string
	// // not implemented
	// components?: [...string]
	// //// not implemented
	// files?: _
	// // not implemented
	// payload_json?: string
	// // not implemented
	// attachments?: [...string]
	// // not implemented
	// flags?: int
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-footer-structure
#EmbedFooter: {
	// footer text
	text: string
	//url of footer icon (only supports http(s) and attachments)
	icon_url?: string
	// a proxied url of footer icon
	proxy_icon_url?: string
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-image-structure
#EmbedImage: {
	// source url of image (only supports http(s) and attachments)
	url: string
	// a proxied url of the image
	proxy_url?: string
	// height of image
	height?: int
	// width of image
	width?: int
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-thumbnail-structure
#EmbedThumbnail: {
	// source url of thumbnail (only supports http(s) and attachments)
	url: string
	// a proxied url of the thumbnail
	proxy_url?: string
	// height of thumbnail
	height?: int
	// width of thumbnail
	width?: int
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-video-structure
#EmbedVideo: {
	// source url of video
	url?: string
	// a proxied url of the video
	proxy_url?: string
	//height of video
	height?: int
	//width of video
	width?: int
}

// API :https://discord.com/developers/docs/resources/channel#embed-object-embed-provider-structure
#EmbedProvider: {
	// name of provider
	name?: string
	//url of provider
	url?: string
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-author-structure
#EmbedAuthor: {
	// name of author
	name: string
	//url of author
	url?: string
	//url of author icon (only supports http(s) and attachments)
	icon_url?: string
	//a proxied url of author icon
	proxy_icon_url?: string
}

// API: https://discord.com/developers/docs/resources/channel#embed-object-embed-field-structure
#EmbedField: {
	// name of the field
	name: string
	//value of the field
	value: string
	//whether or not this field should display inline
	inline?: bool
}

// API: https://discord.com/developers/docs/resources/channel#embed-object
#Embed: {
	// title of embed
	title?: string
	// type of embed (always "rich" for webhook embeds)
	type?: "rich" | "image" | "video" | "gifv" | "article" | "link"
	// description of embed
	description?: string
	// url of embed
	url?: string
	// timestamp of embed content
	timestamp?: string
	// color code of the embed
	color?: int
	// footer information
	footer?: #EmbedFooter
	// image information
	image?: #EmbedImage
	// thumbnail information
	thumbnail?: #EmbedThumbnail
	// video information
	video?: #EmbedVideo
	// provider information
	provider?: #EmbedProvider
	// author information
	author?: #EmbedAuthor
	// fields information
	fields?: [...#EmbedField]
}
