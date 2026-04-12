export interface request{
    hookId:number | string
    router:string
    pagename:string
    payload?:unknown
}

export interface response{
    hookId:number | string
    html:string
    router:string
    pagename:string
    error?:string
    duration?:number
}
